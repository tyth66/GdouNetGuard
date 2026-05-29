package campus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CredentialLoader is a function that returns credentials on demand.
// The caller must invoke creds.Clear() when the credentials are no longer needed.
type CredentialLoader func() (*Credentials, error)

// httpClient wraps HTTP configuration shared across portal requests.
type httpClient struct {
	baseURL string
	http    *http.Client
}

// Guard monitors campus network status and performs automatic re-authentication.
type Guard struct {
	cfg              Config
	credLoader       CredentialLoader
	credentialSource string
	hasCreds         bool
	probeFailCount   int
	portal           *httpClient
}

// NewGuard creates a new Guard instance.
func NewGuard(cfg Config, credLoader CredentialLoader, credSource string, hasCreds bool) *Guard {
	return &Guard{
		cfg:              cfg,
		credLoader:       credLoader,
		credentialSource: credSource,
		hasCreds:         hasCreds,
		portal: &httpClient{
			baseURL: cfg.BaseURL,
			http: &http.Client{
				Timeout: cfg.Timeout,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
		},
	}
}

// HasCreds reports whether the guard has credentials available for login.
func (g *Guard) HasCreds() bool {
	return g.hasCreds
}

// EnsureConnected checks campus status and performs login if needed.
func (g *Guard) EnsureConnected(ctx context.Context, logger *log.Logger) error {
	online, onlineInfo, err := campusOnline(ctx, g.portal)
	if err != nil {
		logger.Printf("campus status unavailable: %v", err)
		if g.cfg.SSID != "" {
			if reconnectErr := reconnectWLAN(g.cfg.SSID, logger); reconnectErr != nil {
				if isProfileNotFound(reconnectErr) {
					logger.Printf("*** WLAN profile %q not found \u2014 reconnect once to this WiFi in Windows to restore the profile ***", g.cfg.SSID)
				} else {
					logger.Printf("wlan reconnect failed: %v", reconnectErr)
				}
			}
			// DHCP and portal may need time after a WLAN reconnect
			time.Sleep(5 * time.Second)
			online, onlineInfo, retryErr := campusOnline(ctx, g.portal)
			if retryErr != nil {
				time.Sleep(3 * time.Second)
				online, onlineInfo, retryErr = campusOnline(ctx, g.portal)
				if retryErr != nil {
					logger.Printf("campus still unavailable after reconnect+retry: %v", retryErr)
					return retryErr
				}
			}
			if online {
				if internetReachable(ctx, g.portal, g.cfg.ProbeURL, g.cfg.ProbeContains) {
					g.probeFailCount = 0
					logger.Printf("online; campus_ip=%s user=%s", firstNonEmpty(onlineInfo.OnlineIP, onlineInfo.ClientIP), onlineInfo.UserName)
					return nil
				}
				return g.handleProbeFail(ctx, logger, onlineInfo)
			}
		} else {
			return err
		}
	} else if online {
		if internetReachable(ctx, g.portal, g.cfg.ProbeURL, g.cfg.ProbeContains) {
			g.probeFailCount = 0
			logger.Printf("online; campus_ip=%s user=%s", firstNonEmpty(onlineInfo.OnlineIP, onlineInfo.ClientIP), onlineInfo.UserName)
			return nil
		}
		return g.handleProbeFail(ctx, logger, onlineInfo)
	}

	if !g.hasCreds {
		return errors.New("no credentials available; set CAMPUS_USERNAME and CAMPUS_PASSWORD, or run -save-credentials first")
	}

	g.probeFailCount = 0
	return g.DoLogin(ctx, logger)
}

// Reauth performs a forced logout-then-login cycle.
func (g *Guard) Reauth(ctx context.Context, logger *log.Logger) error {
	if !g.hasCreds {
		return errors.New("no credentials available for reauth")
	}

	creds, err := g.credLoader()
	if err != nil {
		return fmt.Errorf("reauth: load credentials: %w", err)
	}
	defer creds.Clear()

	ip, err := detectClientIP(ctx, g.portal, g.cfg.ACID, creds.Username)
	if err != nil {
		return fmt.Errorf("reauth: detect IP: %w", err)
	}
	if ip == "" {
		return errors.New("reauth: could not detect client IP")
	}

	logger.Printf("reauth: logging out (ip=%s)", ip)
	if err := portalLogout(ctx, g.portal, creds.Username+g.cfg.Domain, ip, g.cfg.ACID); err != nil {
		logger.Printf("reauth: logout failed: %v", err)
	} else {
		logger.Printf("reauth: logout ok")
		time.Sleep(1 * time.Second)
	}

	return g.DoLogin(ctx, logger)
}

// DoLogin detects the client IP and submits a login request.
// Credentials are loaded on demand and cleared before the function returns.
func (g *Guard) DoLogin(ctx context.Context, logger *log.Logger) error {
	if !g.hasCreds {
		return errors.New("no credentials available for login")
	}

	creds, err := g.credLoader()
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}
	defer creds.Clear()

	ip, err := detectClientIP(ctx, g.portal, g.cfg.ACID, creds.Username)
	if err != nil {
		return err
	}
	if ip == "" {
		return errors.New("could not detect portal client IP")
	}

	// Agree to portal protocol before login (required when UserAgreeSwitch is enabled)
	if err := agreePortalProtocol(ctx, g.portal, creds.Username); err != nil {
		logger.Printf("protocol agreement failed (non-fatal): %v", err)
	} else {
		logger.Printf("portal protocol agreed")
	}

	logger.Printf("offline; attempting SRUN login for ip=%s credentials=%s", ip, g.credentialSource)
	resp, err := portalLogin(ctx, g.portal, creds.Username+g.cfg.Domain, creds.Password, ip, g.cfg.ACID)
	if err != nil {
		return err
	}
	if g.cfg.DryRun {
		logger.Printf("dry-run: login parameters built but not submitted")
		return nil
	}
	if responseOK(resp.Error, resp.Res) || resp.SucMsg != "" {
		logger.Printf("login accepted: res=%s error=%s suc_msg=%s", resp.Res, resp.Error, resp.SucMsg)
		return nil
	}
	return fmt.Errorf("login rejected: res=%s error=%s ecode=%v message=%s", resp.Res, resp.Error, resp.ECode, resp.ErrorMsg)
}

func (g *Guard) handleProbeFail(ctx context.Context, logger *log.Logger, onlineInfo userInfoResponse) error {
	g.probeFailCount++

	if g.cfg.MaxProbeFails > 0 && g.probeFailCount >= g.cfg.MaxProbeFails {
		logger.Printf("internet probe failed %d times consecutively; forcing re-auth", g.probeFailCount)
		if g.hasCreds {
			if err := g.Reauth(ctx, logger); err != nil {
				return err
			}
			g.probeFailCount = 0
			return nil
		}
		logger.Printf("no credentials; cannot force re-auth")
		return nil
	}

	logger.Printf("campus reports online, but internet probe failed (%d/%d); skip reauth this round",
		g.probeFailCount, g.cfg.MaxProbeFails)
	return nil
}

func reconnectWLAN(ssid string, logger *log.Logger) error {
	if runtime.GOOS != "windows" {
		return nil
	}
	logger.Printf("attempting WLAN reconnect: ssid=%s", ssid)
	cmd := exec.Command("netsh", "wlan", "connect", "name="+ssid)
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		logger.Printf("netsh: %s", strings.TrimSpace(string(output)))
	}
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func isProfileNotFound(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	// Match both Chinese and English netsh profile-not-found output
	msg := err.Error()
	return strings.Contains(msg, "未找到配置文件") ||
		strings.Contains(msg, "is not found on any wireless interface")
}

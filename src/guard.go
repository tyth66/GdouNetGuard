package campus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CredentialLoader is a function that returns credentials on demand.
// The caller must invoke creds.Clear() when the credentials are no longer needed.
type CredentialLoader func() (*Credentials, error)

// Guard monitors campus network status and performs automatic re-authentication.
type Guard struct {
	cfg              Config
	credLoader       CredentialLoader
	credentialSource string
	hasCreds         bool
	probeFailCount   int
	portal           *portalClient
	agreementStatus  string // "unknown", "agreed", "disabled", "error"
}

// NewGuard creates a new Guard instance.
func NewGuard(cfg Config, credLoader CredentialLoader, credSource string, hasCreds bool) *Guard {
	return &Guard{
		cfg:              cfg,
		credLoader:       credLoader,
		credentialSource: credSource,
		hasCreds:         hasCreds,
		portal:           newPortalClient(cfg.BaseURL, cfg.Timeout, cfg.RetryMax, cfg.RetryBaseDelay),
	}
}

// HasCreds reports whether the guard has credentials available for login.
func (g *Guard) HasCreds() bool {
	return g.hasCreds
}

// probeContext returns a context with the probe timeout applied.
func (g *Guard) probeContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, g.cfg.ProbeTimeout)
}

// EnsureConnected checks campus status and performs login if needed.
func (g *Guard) EnsureConnected(ctx context.Context, logger *log.Logger) error {
	pctx, cancel := g.probeContext(ctx)
	online, onlineInfo, err := g.portal.campusOnline(pctx)
	cancel()

	if err != nil {
		return g.handleUnreachable(ctx, logger, err)
	}
	if online {
		return g.handleOnline(ctx, logger, onlineInfo)
	}
	return g.handleOffline(ctx, logger)
}

// handleUnreachable reacts to a campusOnline probe error. When a WLAN SSID is
// configured it attempts a netsh reconnect followed by up to two retries.
func (g *Guard) handleUnreachable(ctx context.Context, logger *log.Logger, probeErr error) error {
	logger.Printf("campus status unavailable: %v", probeErr)
	if g.cfg.SSID == "" {
		return probeErr
	}

	if reconnectErr := reconnectWLAN(g.cfg.SSID, logger); reconnectErr != nil {
		if isProfileNotFound(reconnectErr) {
			logger.Printf("*** WLAN profile %q not found \u2014 reconnect once to this WiFi in Windows to restore the profile ***", g.cfg.SSID)
		} else {
			logger.Printf("wlan reconnect failed: %v", reconnectErr)
		}
	}

	// DHCP and portal may need time after a WLAN reconnect
	time.Sleep(5 * time.Second)

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(3 * time.Second)
		}
		pctx, cancel := g.probeContext(ctx)
		online, onlineInfo, retryErr := g.portal.campusOnline(pctx)
		cancel()
		if retryErr != nil {
			continue
		}
		if online {
			return g.handleOnline(ctx, logger, onlineInfo)
		}
		return g.handleOffline(ctx, logger)
	}

	return fmt.Errorf("campus still unreachable after WLAN reconnect: %w", probeErr)
}

// handleOnline reacts to a positive campusOnline result. It verifies actual
// internet connectivity via the probe URL and delegates to handleProbeFail on
// repeated failures.
func (g *Guard) handleOnline(ctx context.Context, logger *log.Logger, onlineInfo userInfoResponse) error {
	pctx, cancel := g.probeContext(ctx)
	reachable := g.portal.internetReachable(pctx, g.cfg.ProbeURLs, g.cfg.ProbeContains)
	cancel()
	if reachable {
		g.probeFailCount = 0
		logger.Printf("online; campus_ip=%s user=%s", firstNonEmpty(onlineInfo.OnlineIP, onlineInfo.ClientIP), onlineInfo.UserName)
		return nil
	}
	return g.handleProbeFail(ctx, logger, onlineInfo)
}

// handleOffline reacts to a negative campusOnline result (portal reachable but
// user is not logged in). It triggers a full login when credentials are available.
func (g *Guard) handleOffline(ctx context.Context, logger *log.Logger) error {
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

	ip, err := g.portal.detectClientIP(ctx, g.cfg.ACID, creds.Username)
	if err != nil {
		return fmt.Errorf("reauth: detect IP: %w", err)
	}
	if ip == "" {
		return errors.New("reauth: could not detect client IP")
	}

	logger.Printf("reauth: logging out (ip=%s)", ip)
	if err := g.portal.portalLogout(ctx, creds.Username+g.cfg.Domain, ip, g.cfg.ACID); err != nil {
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

	ip, err := g.portal.detectClientIP(ctx, g.cfg.ACID, creds.Username)
	if err != nil {
		return err
	}
	if ip == "" {
		return errors.New("could not detect portal client IP")
	}

	// Agree to portal protocol before login (required when UserAgreeSwitch is enabled)
	if err := g.portal.agreePortalProtocol(ctx, creds.Username); err != nil {
		logger.Printf("protocol agreement failed (non-fatal): %v", err)
	} else {
		logger.Printf("portal protocol agreed")
	}

	logger.Printf("offline; attempting SRUN login for ip=%s credentials=%s", ip, g.credentialSource)
	resp, err := g.portal.portalLogin(ctx, creds.Username+g.cfg.Domain, creds.Password, ip, g.cfg.ACID)
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

// ErrWLANNotSupported is returned by reconnectWLAN on non-Windows platforms.
var ErrWLANNotSupported = errors.New("wlan reconnect requires Windows; netsh is not available on this OS")

func reconnectWLAN(ssid string, logger *log.Logger) error {
	if runtime.GOOS != "windows" {
		return ErrWLANNotSupported
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
	msg := err.Error()
	// Exit code 1 combined with a profile-not-found message.
	// netsh messages (any locale):
	//   "There is no profile ... assigned to the specified interface."
	//   "The profile ... is not found on any wireless interface."
	if exitErr.ExitCode() == 1 {
		if strings.Contains(msg, "assigned to the specified interface") ||
			strings.Contains(msg, "is not found on any wireless") ||
			strings.Contains(msg, "no profile") {
			return true
		}
	}
	return false
}


// classifyAgreementError categorizes agreement errors for observability.
func classifyAgreementError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "returned HTTP 404") {
		return "disabled"
	}
	if strings.Contains(msg, "protocol unavailable") {
		return "unavailable"
	}
	return "error"
}

package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"GdouNetGuard/src"
)

func main() {
	cfg := campus.ParseFlags()

	if cfg.PidFile == "" {
		cfg.PidFile = filepath.Join(os.TempDir(), "GdouNetGuard.pid")
	}

	campus.RotateIfNeeded(cfg.LogFile, cfg.LogMaxSize, cfg.LogMaxBackups)
	campus.CleanupOldLogs(cfg.LogFile, cfg.LogMaxAge)

	var logger *log.Logger
	if cfg.LogFile != "" {
		rw, err := campus.NewRotatingWriter(cfg.LogFile, cfg.LogMaxSize, cfg.LogMaxBackups)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cannot open log file %s: %v\n", cfg.LogFile, err)
			os.Exit(1)
		}
		dw := campus.NewDedupWriter(rw)
		defer dw.Close()
		logger = log.New(dw, "", log.LstdFlags)
	} else {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	if err := cfg.Validate(); err != nil {
		logger.Fatal(err)
	}

	store, err := campus.NewCredentialStore(cfg.CredentialFile)
	if err != nil {
		logger.Fatal(err)
	}

	// Exit-early flags (setup operations)
	if cfg.ForgetCredentials {
		if err := store.Delete(); err != nil {
			logger.Fatal(err)
		}
		logger.Printf("saved credentials removed: %s", store.Path())
		return
	}
	if cfg.SaveCredentials {
		if err := campus.SaveCredentialsFromEnv(cfg, store); err != nil {
			logger.Printf("environment variables not set, prompting interactively")
			creds, promptErr := interactivePromptCredentials(cfg.UsernameEnv, cfg.PasswordEnv)
			if promptErr != nil {
				logger.Fatal(promptErr)
			}
			if err := store.Save(creds); err != nil {
				logger.Fatal(err)
			}
		}
		logger.Printf("credentials saved with Windows DPAPI: %s", store.Path())
		if err := campus.SaveConfigFile(cfg, cfg.ConfigFile); err != nil {
			logger.Printf("config save failed: %v", err)
		} else {
			logger.Printf("config saved: %s", cfg.ConfigFile)
		}
		return
	}
	if cfg.DisableStartup {
		if err := campus.DisableStartup(cfg.StartupTaskName); err != nil {
			logger.Fatal(err)
		}
		logger.Printf("startup task disabled: %s", cfg.StartupTaskName)
		return
	}
	if cfg.EnableStartup {
		if err := campus.EnableStartup(cfg); err != nil {
			logger.Fatal(err)
		}
		logger.Printf("startup task enabled: %s", cfg.StartupTaskName)
		if err := campus.SaveConfigFile(cfg, cfg.ConfigFile); err != nil {
			logger.Printf("config save failed: %v", err)
		} else {
			logger.Printf("config saved: %s", cfg.ConfigFile)
		}
		return
	}
	if cfg.Background {
		if err := campus.SaveConfigFile(cfg, cfg.ConfigFile); err != nil {
			logger.Printf("config save failed: %v", err)
		}
		if err := campus.StartBackground(cfg); err != nil {
			logger.Fatal(err)
		}
		logger.Printf("campus auth guard started in background")
		return
	}

	pidLock, err := acquirePidFile(cfg.PidFile)
	if err != nil {
		logger.Fatal(err)
	}
	defer func() {
		pidLock.Close()
		os.Remove(cfg.PidFile)
	}()

	creds, credSource, hasCreds := resolveCredentials(cfg, store, logger)

	// First startup: if no credentials exist and we are running as a foreground
	// daemon (not -once/ -reauth/ -background), prompt interactively and auto-save.
	if !hasCreds && !cfg.Once && !cfg.Reauth && !cfg.Background {
		logger.Printf("*** No credentials found. Set %s and %s, or enter them now. ***", cfg.UsernameEnv, cfg.PasswordEnv)
		promptedCreds, err := interactivePromptCredentials(cfg.UsernameEnv, cfg.PasswordEnv)
		if err != nil {
			logger.Printf("interactive prompt failed: %v", err)
		} else {
			if err := store.Save(promptedCreds); err != nil {
				logger.Fatalf("auto-save after prompt failed: %v", err)
			}
			logger.Printf("credentials auto-saved to %s", store.Path())
			creds = &promptedCreds
			credSource = "interactive prompt"
			hasCreds = true
		}
	}

	// Auto-save credentials from environment to DPAPI store, so future runs
	// can authenticate without environment variables being set.
	if hasCreds && credSource == "environment" {
		if err := store.Save(*creds); err != nil {
			logger.Printf("auto-save credentials failed: %v", err)
		} else {
			logger.Printf("credentials auto-saved to %s", store.Path())
		}
	}

	// Save effective config for future runs (background, startup, etc).
	if err := campus.SaveConfigFile(cfg, cfg.ConfigFile); err != nil {
		logger.Printf("config save failed: %v", err)
	}

	credLoader := func() (*campus.Credentials, error) {
		c, _, err := campus.LoadCredentials(cfg, store)
		if err != nil {
			return nil, err
		}
		return &c, nil
	}
	guard := campus.NewGuard(cfg, credLoader, credSource, hasCreds)

	// -reauth: logout then login, one-shot
	if cfg.Reauth {
		if !guard.HasCreds() {
			logger.Fatal("no credentials available for -reauth")
		}
		if err := guard.Reauth(context.Background(), logger); err != nil {
			logger.Fatal(err)
		}
		return
	}

	if cfg.Once {
		if err := guard.EnsureConnected(context.Background(), logger); err != nil {
			logger.Fatal(err)
		}
		return
	}

	startupMsg := fmt.Sprintf("campus auth guard started; interval=%s base=%s ac_id=%s", cfg.Interval, cfg.BaseURL, cfg.ACID)
	if cfg.SSID != "" {
		startupMsg += fmt.Sprintf(" ssid=%s", cfg.SSID)
	}
	if cfg.LogFile != "" {
		startupMsg += fmt.Sprintf(" log-file=%s", cfg.LogFile)
	}
	if cfg.MaxProbeFails > 0 {
		startupMsg += fmt.Sprintf(" max-probe-fails=%d", cfg.MaxProbeFails)
	}
	logger.Print(startupMsg)
	// Warn when WLAN reconnect is enabled but auto-auth is unavailable.
	if !hasCreds && cfg.SSID != "" {
		logger.Print("*** WLAN reconnect is active but auto-auth is unavailable: save credentials with -save-credentials to enable full recovery ***")
	}
	// Drop the startup credential load now that we know the source is valid.
	// Subsequent authentication rounds will reload credentials on demand.
	_ = creds
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if err := guard.EnsureConnected(context.Background(), logger); err != nil {
		logger.Printf("check failed: %v", err)
	}

	for {
		select {
		case sig := <-sigCh:
			logger.Printf("received %v, shutting down", sig)
			return
		case <-ticker.C:
			if err := guard.EnsureConnected(context.Background(), logger); err != nil {
				logger.Printf("check failed: %v", err)
			}
		}
	}
}

func acquirePidFile(path string) (*os.File, error) {
	if existing, err := os.ReadFile(path); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(existing), "%d", &pid); scanErr == nil {
			if proc, findErr := os.FindProcess(pid); findErr == nil {
				_ = proc
				return nil, fmt.Errorf("another guard instance is already running (pid %d); if this is stale, delete %s", pid, path)
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("create pid file %s: %v", path, err)
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func resolveCredentials(cfg campus.Config, store campus.CredentialStore, logger *log.Logger) (*campus.Credentials, string, bool) {
	creds, source, err := campus.LoadCredentials(cfg, store)
	if err != nil {
		logger.Printf("credentials not loaded: %v", err)
		return nil, "", false
	}
	logger.Printf("credentials loaded from %s", source)
	return &creds, source, true
}

// interactivePromptCredentials reads username and password from stdin.
// The env names are shown in the prompts so the user knows which variables
// would be used instead of interactive input.
func interactivePromptCredentials(usernameEnv, passwordEnv string) (campus.Credentials, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Printf("Campus username (%s): ", usernameEnv)
	username, err := reader.ReadString('\n')
	if err != nil {
		return campus.Credentials{}, fmt.Errorf("read username: %w", err)
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return campus.Credentials{}, errors.New("username is required")
	}

	fmt.Printf("Campus password (%s): ", passwordEnv)
	password, err := reader.ReadString('\n')
	if err != nil {
		return campus.Credentials{}, fmt.Errorf("read password: %w", err)
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return campus.Credentials{}, errors.New("password is required")
	}

	return campus.Credentials{Username: username, Password: password}, nil
}

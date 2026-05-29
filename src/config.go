package campus

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	DefaultBaseURL         = "http://10.129.1.1"
	DefaultACID            = "153"
	DefaultProbeURL        = "http://www.msftconnecttest.com/connecttest.txt"
	DefaultProbeContains   = "Microsoft Connect Test"
	DefaultCallback        = "campusAuth"
	DefaultStartupTaskName = "GdouNetGuard"
	DefaultMaxProbeFails   = 3
	DefaultLogMaxSize      = 1 << 20 // 1 MB
	DefaultLogMaxBackups   = 3
)

// Config holds all application configuration.
type Config struct {
	BaseURL           string
	ACID              string
	Domain            string
	SSID              string
	ProbeURL          string
	ProbeContains     string
	UsernameEnv       string
	PasswordEnv       string
	CredentialFile    string
	Interval          time.Duration
	Timeout           time.Duration
	Once              bool
	DryRun            bool
	Reauth            bool
	SaveCredentials   bool
	ForgetCredentials bool
	EnableStartup     bool
	DisableStartup    bool
	Background        bool
	StartupTaskName   string
	LogFile           string
	LogMaxSize        int64
	LogMaxBackups     int
	PidFile           string
	MaxProbeFails     int
}

// ParseFlags parses command-line flags and returns a Config.
func ParseFlags() Config {
	cfg := Config{}
	flag.StringVar(&cfg.BaseURL, "base-url", DefaultBaseURL, "campus portal base URL")
	flag.StringVar(&cfg.ACID, "ac-id", DefaultACID, "SRUN ac_id")
	flag.StringVar(&cfg.Domain, "domain", "", "optional account domain suffix, such as @cmcc")
	flag.StringVar(&cfg.SSID, "ssid", "海大校园网", "WLAN profile name for netsh wlan connect")
	flag.StringVar(&cfg.ProbeURL, "probe-url", DefaultProbeURL, "internet connectivity probe URL")
	flag.StringVar(&cfg.ProbeContains, "probe-contains", DefaultProbeContains, "text expected in probe response; empty disables body check")
	flag.StringVar(&cfg.UsernameEnv, "username-env", "CAMPUS_USERNAME", "environment variable containing campus username")
	flag.StringVar(&cfg.PasswordEnv, "password-env", "CAMPUS_PASSWORD", "environment variable containing campus password")
	flag.StringVar(&cfg.CredentialFile, "credential-file", "", "encrypted credential store path; defaults to the current user's config directory")
	flag.DurationVar(&cfg.Interval, "interval", 15*time.Second, "guard loop interval")
	flag.DurationVar(&cfg.Timeout, "timeout", 8*time.Second, "HTTP timeout")
	flag.BoolVar(&cfg.Once, "once", false, "run one check/login attempt and exit")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "build login parameters but do not submit them")
	flag.BoolVar(&cfg.Reauth, "reauth", false, "force logout then login and exit")
	flag.BoolVar(&cfg.SaveCredentials, "save-credentials", false, "save credentials from environment variables with Windows DPAPI and exit")
	flag.BoolVar(&cfg.ForgetCredentials, "forget-credentials", false, "delete saved credentials and exit")
	flag.BoolVar(&cfg.EnableStartup, "enable-startup", false, "create or update the current user's startup task and exit")
	flag.BoolVar(&cfg.DisableStartup, "disable-startup", false, "delete the current user's startup task and exit")
	flag.BoolVar(&cfg.Background, "background", false, "start the guard in a hidden background process and exit")
	flag.StringVar(&cfg.StartupTaskName, "startup-task-name", DefaultStartupTaskName, "Windows scheduled task name for startup")
	flag.StringVar(&cfg.LogFile, "log-file", "", "log file path; omitted by default (writes to stdout)")
	flag.Int64Var(&cfg.LogMaxSize, "log-max-size", DefaultLogMaxSize, "rotate log when it exceeds this many bytes")
	flag.IntVar(&cfg.LogMaxBackups, "log-max-backups", DefaultLogMaxBackups, "number of rotated log backups to keep")
	flag.StringVar(&cfg.PidFile, "pid-file", "", "PID file for mutual exclusion; defaults to os.TempDir")
	flag.IntVar(&cfg.MaxProbeFails, "max-probe-fails", DefaultMaxProbeFails, "consecutive internet probe failures before forcing re-auth; 0 disables")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(Version)
		os.Exit(0)
	}
	return cfg
}

// Validate checks the config for errors.
func (c Config) Validate() error {
	if c.BaseURL == "" {
		return errors.New("-base-url is required")
	}
	if c.ACID == "" {
		return errors.New("-ac-id is required")
	}
	if c.Interval <= 0 {
		return errors.New("-interval must be positive")
	}
	if c.Timeout <= 0 {
		return errors.New("-timeout must be positive")
	}
	setupFlagCount := 0
	for _, enabled := range []bool{c.SaveCredentials, c.ForgetCredentials, c.EnableStartup, c.DisableStartup, c.Reauth} {
		if enabled {
			setupFlagCount++
		}
	}
	if setupFlagCount > 1 {
		return errors.New("setup/one-shot flags cannot be combined")
	}
	if c.Background && c.Once {
		return errors.New("-background cannot be used with -once")
	}
	if c.Background && (c.SaveCredentials || c.ForgetCredentials || c.EnableStartup || c.DisableStartup || c.Reauth) {
		return errors.New("-background cannot be combined with setup/one-shot flags")
	}
	if c.EnableStartup && c.Once {
		return errors.New("-enable-startup cannot be used with -once")
	}
	if (c.EnableStartup || c.DisableStartup) && strings.TrimSpace(c.StartupTaskName) == "" {
		return errors.New("-startup-task-name is required")
	}
	if c.Reauth && c.Once {
		return errors.New("-reauth cannot be used with -once")
	}
	if c.LogMaxSize < 0 {
		return errors.New("-log-max-size cannot be negative")
	}
	if c.LogMaxBackups < 0 {
		return errors.New("-log-max-backups cannot be negative")
	}
	if c.MaxProbeFails < 0 {
		return errors.New("-max-probe-fails cannot be negative")
	}
	if _, err := url.ParseRequestURI(c.BaseURL); err != nil {
		return fmt.Errorf("invalid -base-url: %w", err)
	}
	if c.ProbeURL != "" {
		if _, err := url.ParseRequestURI(c.ProbeURL); err != nil {
			return fmt.Errorf("invalid -probe-url: %w", err)
		}
	}
	return nil
}

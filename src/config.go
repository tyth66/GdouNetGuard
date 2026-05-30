package campus

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	DefaultInterval        = 30 * time.Second
	DefaultProbeTimeout    = 30 * time.Second
	DefaultLogMaxAge       = 7 * 24 * time.Hour
	DefaultRetryMax       = 2
	DefaultRetryBaseDelay = 500 * time.Millisecond
	DefaultSSID            = "海大校园网"
	DefaultUsernameEnv     = "CAMPUS_USERNAME"
	DefaultPasswordEnv     = "CAMPUS_PASSWORD"
	DefaultTimeout         = 8 * time.Second
)

// Config holds all application configuration.
type Config struct {
	BaseURL           string
	ACID              string
	Domain            string
	SSID              string
	ProbeURLs         []string
	ProbeContains     string
	UsernameEnv       string
	PasswordEnv       string
	CredentialFile    string
	ConfigFile        string
	Interval          time.Duration
	Timeout           time.Duration
	ProbeTimeout      time.Duration
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
	RetryMax         int
	RetryBaseDelay   time.Duration
	LogMaxAge         time.Duration
}

// configFilePayload is the JSON-serializable subset of Config.
type configFilePayload struct {
	BaseURL        string `json:"base_url,omitempty"`
	ACID           string `json:"ac_id,omitempty"`
	Domain         string `json:"domain,omitempty"`
	SSID           string `json:"ssid,omitempty"`
	ProbeURLs      string `json:"probe_urls,omitempty"`
	ProbeContains  string `json:"probe_contains,omitempty"`
	UsernameEnv    string `json:"username_env,omitempty"`
	PasswordEnv    string `json:"password_env,omitempty"`
	CredentialFile string `json:"credential_file,omitempty"`
	Interval       string `json:"interval,omitempty"`
	Timeout        string `json:"timeout,omitempty"`
	ProbeTimeout   string `json:"probe_timeout,omitempty"`
	StartupTaskName string `json:"startup_task_name,omitempty"`
	LogFile        string `json:"log_file,omitempty"`
	LogMaxSize     int64  `json:"log_max_size,omitempty"`
	LogMaxBackups  int    `json:"log_max_backups,omitempty"`
	MaxProbeFails  int    `json:"max_probe_fails,omitempty"`
	RetryMax       int    `json:"retry_max,omitempty"`
	RetryBaseDelay string `json:"retry_base_delay,omitempty"`
	LogMaxAge      string `json:"log_max_age,omitempty"`
}

// DefaultConfigFilePath returns the default config file path.
func DefaultConfigFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "GdouNetGuard", "config.json"), nil
}

// DefaultLogFilePath returns the default log file path. The directory is the
// same as the config file directory.
func DefaultLogFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "GdouNetGuard", "guard.log"), nil
}

// LoadConfigFile reads a JSON config file and returns the parsed settings.
// Missing or empty fields are left at their zero value.
func LoadConfigFile(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	var p configFilePayload
	if err := json.Unmarshal(data, &p); err != nil {
		return cfg, fmt.Errorf("decode config file %s: %w", path, err)
	}
	cfg.BaseURL = p.BaseURL
	cfg.ACID = p.ACID
	cfg.Domain = p.Domain
	cfg.SSID = p.SSID
	if p.ProbeURLs != "" {
		cfg.ProbeURLs = strings.Split(p.ProbeURLs, ",")
	}
	cfg.ProbeContains = p.ProbeContains
	cfg.UsernameEnv = p.UsernameEnv
	cfg.PasswordEnv = p.PasswordEnv
	cfg.CredentialFile = p.CredentialFile
	cfg.StartupTaskName = p.StartupTaskName
	cfg.LogFile = p.LogFile
	cfg.LogMaxSize = p.LogMaxSize
	cfg.LogMaxBackups = p.LogMaxBackups
	cfg.MaxProbeFails = p.MaxProbeFails
	if p.LogMaxAge != "" {
		cfg.LogMaxAge, _ = time.ParseDuration(p.LogMaxAge)
	}
	if p.Interval != "" {
		cfg.Interval, _ = time.ParseDuration(p.Interval)
	}
	if p.Timeout != "" {
		cfg.Timeout, _ = time.ParseDuration(p.Timeout)
	}
	if p.ProbeTimeout != "" {
		cfg.ProbeTimeout, _ = time.ParseDuration(p.ProbeTimeout)
	}
	return cfg, nil
}

// SaveConfigFile writes the effective config to a JSON file.
// Only non-empty / non-zero fields that differ from defaults are persisted.
func SaveConfigFile(cfg Config, path string) error {
	p := configFilePayload{}
	def := defaultConfig()
	if cfg.BaseURL != def.BaseURL {
		p.BaseURL = cfg.BaseURL
	}
	if cfg.ACID != def.ACID {
		p.ACID = cfg.ACID
	}
	if cfg.Domain != def.Domain {
		p.Domain = cfg.Domain
	}
	if cfg.SSID != def.SSID {
		p.SSID = cfg.SSID
	}
	if !stringSlicesEqual(cfg.ProbeURLs, def.ProbeURLs) {
		p.ProbeURLs = strings.Join(cfg.ProbeURLs, ",")
	}
	if cfg.ProbeContains != def.ProbeContains {
		p.ProbeContains = cfg.ProbeContains
	}
	if cfg.UsernameEnv != def.UsernameEnv {
		p.UsernameEnv = cfg.UsernameEnv
	}
	if cfg.PasswordEnv != def.PasswordEnv {
		p.PasswordEnv = cfg.PasswordEnv
	}
	if cfg.CredentialFile != def.CredentialFile {
		p.CredentialFile = cfg.CredentialFile
	}
	if cfg.StartupTaskName != def.StartupTaskName {
		p.StartupTaskName = cfg.StartupTaskName
	}
	if cfg.LogFile != def.LogFile {
		p.LogFile = cfg.LogFile
	}
	if cfg.LogMaxSize != def.LogMaxSize {
		p.LogMaxSize = cfg.LogMaxSize
	}
	if cfg.LogMaxBackups != def.LogMaxBackups {
		p.LogMaxBackups = cfg.LogMaxBackups
	}
	if cfg.MaxProbeFails != def.MaxProbeFails {
		p.MaxProbeFails = cfg.MaxProbeFails
	}
	if cfg.RetryMax != def.RetryMax {
		p.RetryMax = cfg.RetryMax
	}
	if cfg.RetryBaseDelay != def.RetryBaseDelay {
		p.RetryBaseDelay = cfg.RetryBaseDelay.String()
	}
	if cfg.LogMaxAge != def.LogMaxAge {
		p.LogMaxAge = cfg.LogMaxAge.String()
	}
	if cfg.Interval != def.Interval {
		p.Interval = cfg.Interval.String()
	}
	if cfg.Timeout != def.Timeout {
		p.Timeout = cfg.Timeout.String()
	}
	if cfg.ProbeTimeout != def.ProbeTimeout {
		p.ProbeTimeout = cfg.ProbeTimeout.String()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// defaultConfig returns a Config populated with all hardcoded defaults.
func defaultConfig() Config {
	return Config{
		BaseURL:        DefaultBaseURL,
		ACID:           DefaultACID,
		Domain:         "",
		SSID:           DefaultSSID,
		ProbeURLs:      []string{DefaultProbeURL},
		ProbeContains:  DefaultProbeContains,
		UsernameEnv:    DefaultUsernameEnv,
		PasswordEnv:    DefaultPasswordEnv,
		CredentialFile: "",
		Interval:       DefaultInterval,
		Timeout:        DefaultTimeout,
		ProbeTimeout:   DefaultProbeTimeout,
		StartupTaskName: DefaultStartupTaskName,
		LogFile:        "",
		LogMaxSize:     DefaultLogMaxSize,
		LogMaxBackups:  DefaultLogMaxBackups,
		MaxProbeFails:  DefaultMaxProbeFails,
		RetryMax:       DefaultRetryMax,
		RetryBaseDelay: DefaultRetryBaseDelay,
		LogMaxAge:      DefaultLogMaxAge,
	}
}

// mergeConfig merges file-based config with CLI overrides.
// CLI values that differ from hardcoded defaults take precedence;
// otherwise file values are used.
func mergeConfig(file, cli Config) Config {
	def := defaultConfig()
	out := cli

	if cli.BaseURL == def.BaseURL && file.BaseURL != "" {
		out.BaseURL = file.BaseURL
	}
	if cli.ACID == def.ACID && file.ACID != "" {
		out.ACID = file.ACID
	}
	if cli.Domain == def.Domain && file.Domain != "" {
		out.Domain = file.Domain
	}
	if cli.SSID == def.SSID && file.SSID != "" {
		out.SSID = file.SSID
	}
	if stringSlicesEqual(cli.ProbeURLs, def.ProbeURLs) && len(file.ProbeURLs) > 0 {
		out.ProbeURLs = file.ProbeURLs
	}
	if cli.ProbeContains == def.ProbeContains && file.ProbeContains != "" {
		out.ProbeContains = file.ProbeContains
	}
	if cli.UsernameEnv == def.UsernameEnv && file.UsernameEnv != "" {
		out.UsernameEnv = file.UsernameEnv
	}
	if cli.PasswordEnv == def.PasswordEnv && file.PasswordEnv != "" {
		out.PasswordEnv = file.PasswordEnv
	}
	if cli.CredentialFile == def.CredentialFile && file.CredentialFile != "" {
		out.CredentialFile = file.CredentialFile
	}
	if cli.StartupTaskName == def.StartupTaskName && file.StartupTaskName != "" {
		out.StartupTaskName = file.StartupTaskName
	}
	if cli.LogFile == def.LogFile && file.LogFile != "" {
		out.LogFile = file.LogFile
	}
	if cli.LogMaxSize == def.LogMaxSize && file.LogMaxSize != 0 {
		out.LogMaxSize = file.LogMaxSize
	}
	if cli.LogMaxBackups == def.LogMaxBackups && file.LogMaxBackups != 0 {
		out.LogMaxBackups = file.LogMaxBackups
	}
	if cli.MaxProbeFails == def.MaxProbeFails && file.MaxProbeFails != 0 {
		out.MaxProbeFails = file.MaxProbeFails
	}
	if cli.RetryMax == def.RetryMax && file.RetryMax != 0 {
		out.RetryMax = file.RetryMax
	}
	if cli.RetryBaseDelay == def.RetryBaseDelay && file.RetryBaseDelay != 0 {
		out.RetryBaseDelay = file.RetryBaseDelay
	}
	if cli.LogMaxAge == def.LogMaxAge && file.LogMaxAge != 0 {
		out.LogMaxAge = file.LogMaxAge
	}
	if cli.Interval == def.Interval && file.Interval != 0 {
		out.Interval = file.Interval
	}
	if cli.Timeout == def.Timeout && file.Timeout != 0 {
		out.Timeout = file.Timeout
	}
	if cli.ProbeTimeout == def.ProbeTimeout && file.ProbeTimeout != 0 {
		out.ProbeTimeout = file.ProbeTimeout
	}

	return out
}

// ParseFlags parses command-line flags and returns a Config.
// Values from the config file (if present) are used as defaults;
// explicit CLI flags override both file and hardcoded defaults.
func ParseFlags() Config {
	cfg := Config{}
	flag.StringVar(&cfg.BaseURL, "base-url", DefaultBaseURL, "campus portal base URL")
	flag.StringVar(&cfg.ACID, "ac-id", DefaultACID, "SRUN ac_id")
	flag.StringVar(&cfg.Domain, "domain", "", "optional account domain suffix, such as @cmcc")
	flag.StringVar(&cfg.SSID, "ssid", DefaultSSID, "WLAN profile name for netsh wlan connect")
	probeURLFlag := ""
	flag.StringVar(&probeURLFlag, "probe-url", DefaultProbeURL, "internet connectivity probe URL; use -probe-urls for multiple")
	var probeURLsCSV string
	flag.StringVar(&probeURLsCSV, "probe-urls", "", "comma-separated internet connectivity probe URLs (overrides -probe-url)")
	flag.StringVar(&cfg.ProbeContains, "probe-contains", DefaultProbeContains, "text expected in probe response; empty disables body check")
	flag.StringVar(&cfg.UsernameEnv, "username-env", DefaultUsernameEnv, "environment variable containing campus username")
	flag.StringVar(&cfg.PasswordEnv, "password-env", DefaultPasswordEnv, "environment variable containing campus password")
	flag.StringVar(&cfg.CredentialFile, "credential-file", "", "encrypted credential store path; defaults to the current user's config directory")
	flag.StringVar(&cfg.ConfigFile, "config", "", "JSON config file path; defaults to %LocalAppData%\\GdouNetGuard\\config.json")
	flag.DurationVar(&cfg.Interval, "interval", DefaultInterval, "guard loop interval")
	flag.DurationVar(&cfg.Timeout, "timeout", DefaultTimeout, "HTTP timeout for login operations")
	flag.DurationVar(&cfg.ProbeTimeout, "probe-timeout", DefaultProbeTimeout, "HTTP timeout for status probes")
	flag.BoolVar(&cfg.Once, "once", false, "run one check/login attempt and exit")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "build login parameters but do not submit them")
	flag.BoolVar(&cfg.Reauth, "reauth", false, "force logout then login and exit")
	flag.BoolVar(&cfg.SaveCredentials, "save-credentials", false, "save credentials from environment variables with Windows DPAPI and exit")
	flag.BoolVar(&cfg.ForgetCredentials, "forget-credentials", false, "delete saved credentials and exit")
	flag.BoolVar(&cfg.EnableStartup, "enable-startup", false, "create or update the current user's startup task and exit")
	flag.BoolVar(&cfg.DisableStartup, "disable-startup", false, "delete the current user's startup task and exit")
	flag.BoolVar(&cfg.Background, "background", false, "start the guard in a hidden background process and exit")
	flag.StringVar(&cfg.StartupTaskName, "startup-task-name", DefaultStartupTaskName, "Windows scheduled task name for startup")
	flag.StringVar(&cfg.LogFile, "log-file", "", `log file path; defaults to %AppData%\GdouNetGuard\guard.log`)
	flag.Int64Var(&cfg.LogMaxSize, "log-max-size", DefaultLogMaxSize, "rotate log when it exceeds this many bytes")
	flag.IntVar(&cfg.LogMaxBackups, "log-max-backups", DefaultLogMaxBackups, "number of rotated log backups to keep")
	flag.DurationVar(&cfg.LogMaxAge, "log-max-age", DefaultLogMaxAge, "delete rotated log backups older than this duration")
	flag.StringVar(&cfg.PidFile, "pid-file", "", "PID file for mutual exclusion; defaults to os.TempDir")
	flag.IntVar(&cfg.MaxProbeFails, "max-probe-fails", DefaultMaxProbeFails, "consecutive internet probe failures before forcing re-auth; 0 disables")
	flag.IntVar(&cfg.RetryMax, "retry-max", DefaultRetryMax, "max HTTP retries per request; 0 disables")
	flag.DurationVar(&cfg.RetryBaseDelay, "retry-base-delay", DefaultRetryBaseDelay, "initial delay for exponential backoff")
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.Parse()
	if showVersion {
		fmt.Println(Version)
		os.Exit(0)
	}

	// Build probe URL list
	if probeURLsCSV != "" {
		for _, u := range strings.Split(probeURLsCSV, ",") {
			if trimmed := strings.TrimSpace(u); trimmed != "" {
				cfg.ProbeURLs = append(cfg.ProbeURLs, trimmed)
			}
		}
	} else {
		cfg.ProbeURLs = []string{probeURLFlag}
	}

	// Resolve config file path
	if cfg.ConfigFile == "" {
		defaultPath, err := DefaultConfigFilePath()
		if err == nil {
			cfg.ConfigFile = defaultPath
		}
	}

	// Resolve default log file path
	if cfg.LogFile == "" {
		defaultPath, err := DefaultLogFilePath()
		if err == nil {
			cfg.LogFile = defaultPath
		}
	}

	// Merge config file values as defaults (CLI overrides win)
	if cfg.ConfigFile != "" {
		if fileCfg, err := LoadConfigFile(cfg.ConfigFile); err == nil {
			cfg = mergeConfig(fileCfg, cfg)
		}
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
	if c.ProbeTimeout <= 0 {
		return errors.New("-probe-timeout must be positive")
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
	if c.RetryMax < 0 {
		return errors.New("-retry-max cannot be negative")
	}
	if c.RetryBaseDelay < 0 {
		return errors.New("-retry-base-delay cannot be negative")
	}
	if _, err := url.ParseRequestURI(c.BaseURL); err != nil {
		return fmt.Errorf("invalid -base-url: %w", err)
	}
	for _, u := range c.ProbeURLs {
		if u != "" {
			if _, err := url.ParseRequestURI(u); err != nil {
				return fmt.Errorf("invalid probe URL %q: %w", u, err)
			}
		}
	}
	return nil
}


func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

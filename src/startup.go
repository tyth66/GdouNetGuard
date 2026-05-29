package campus

import (
	"strconv"
	"time"
)

// GuardArgs returns the CLI arguments needed to re-launch the guard process
// with the same configuration.
func GuardArgs(cfg Config, includeBackground bool) []string {
	args := make([]string, 0, 32)
	if includeBackground {
		args = append(args, "-background")
	}
	args = appendStringFlag(args, "base-url", cfg.BaseURL, DefaultBaseURL)
	args = appendStringFlag(args, "ac-id", cfg.ACID, DefaultACID)
	args = appendStringFlag(args, "domain", cfg.Domain, "")
	args = appendStringFlag(args, "ssid", cfg.SSID, "海大校园网")
	args = appendStringFlag(args, "probe-url", cfg.ProbeURL, DefaultProbeURL)
	args = appendStringFlag(args, "probe-contains", cfg.ProbeContains, DefaultProbeContains)
	args = appendStringFlag(args, "username-env", cfg.UsernameEnv, "CAMPUS_USERNAME")
	args = appendStringFlag(args, "password-env", cfg.PasswordEnv, "CAMPUS_PASSWORD")
	args = appendStringFlag(args, "credential-file", cfg.CredentialFile, "")
	args = appendDurationFlag(args, "interval", cfg.Interval, 15*time.Second)
	args = appendDurationFlag(args, "timeout", cfg.Timeout, 8*time.Second)
	if cfg.DryRun {
		args = append(args, "-dry-run")
	}
	args = appendStringFlag(args, "log-file", cfg.LogFile, "")
	args = appendInt64Flag(args, "log-max-size", cfg.LogMaxSize, DefaultLogMaxSize)
	args = appendIntFlag(args, "log-max-backups", cfg.LogMaxBackups, DefaultLogMaxBackups)
	args = appendStringFlag(args, "pid-file", cfg.PidFile, "")
	args = appendIntFlag(args, "max-probe-fails", cfg.MaxProbeFails, DefaultMaxProbeFails)
	return args
}

func appendStringFlag(args []string, name, value, defaultValue string) []string {
	if value == defaultValue {
		return args
	}
	return append(args, "-"+name, value)
}

func appendDurationFlag(args []string, name string, value, defaultValue time.Duration) []string {
	if value == defaultValue {
		return args
	}
	return append(args, "-"+name, value.String())
}

func appendIntFlag(args []string, name string, value, defaultValue int) []string {
	if value == defaultValue {
		return args
	}
	return append(args, "-"+name, strconv.Itoa(value))
}

func appendInt64Flag(args []string, name string, value, defaultValue int64) []string {
	if value == defaultValue {
		return args
	}
	return append(args, "-"+name, strconv.FormatInt(value, 10))
}

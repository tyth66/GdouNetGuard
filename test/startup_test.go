package campus_test

import (
	"reflect"
	"testing"
	"time"

	"GdouNetGuard/src"
)

func TestGuardArgsIncludeOnlyNonDefaultNonSecretSettings(t *testing.T) {
	cfg := campus.Config{
		BaseURL:        campus.DefaultBaseURL,
		ACID:           campus.DefaultACID,
		SSID:           "Campus-Test",
		ProbeURL:       campus.DefaultProbeURL,
		ProbeContains:  campus.DefaultProbeContains,
		UsernameEnv:    "CAMPUS_USERNAME",
		PasswordEnv:    "CAMPUS_PASSWORD",
		CredentialFile: `C:\Users\Alice\AppData\Local\GdouNetGuard\credentials.json`,
		Interval:       10 * time.Second,
		Timeout:        8 * time.Second,
		MaxProbeFails:  campus.DefaultMaxProbeFails,
		LogMaxSize:     campus.DefaultLogMaxSize,
		LogMaxBackups:  campus.DefaultLogMaxBackups,
	}

	got := campus.GuardArgs(cfg, true)
	want := []string{
		"-background",
		"-ssid", "Campus-Test",
		"-credential-file", `C:\Users\Alice\AppData\Local\GdouNetGuard\credentials.json`,
		"-interval", "10s",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestValidateRejectsConflictingStartupFlags(t *testing.T) {
	cfg := validTestConfig()
	cfg.EnableStartup = true
	cfg.DisableStartup = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateRejectsBackgroundOnce(t *testing.T) {
	cfg := validTestConfig()
	cfg.Background = true
	cfg.Once = true
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error")
	}
}

func validTestConfig() campus.Config {
	return campus.Config{
		BaseURL:         campus.DefaultBaseURL,
		ACID:            campus.DefaultACID,
		ProbeURL:        campus.DefaultProbeURL,
		ProbeContains:   campus.DefaultProbeContains,
		UsernameEnv:     "CAMPUS_USERNAME",
		PasswordEnv:     "CAMPUS_PASSWORD",
		Interval:        15 * time.Second,
		Timeout:         8 * time.Second,
		StartupTaskName: campus.DefaultStartupTaskName,
		MaxProbeFails:   campus.DefaultMaxProbeFails,
		LogMaxSize:      campus.DefaultLogMaxSize,
		LogMaxBackups:   campus.DefaultLogMaxBackups,
	}
}

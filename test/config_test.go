package campus_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"GdouNetGuard/src"
)

func TestSaveAndLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := campus.Config{
		BaseURL:       "http://10.129.1.1",
		ACID:          "200",
		SSID:          "TestWiFi",
		Interval:      45 * time.Second,
		MaxProbeFails: 5,
		LogMaxAge:     72 * time.Hour,
	}

	if err := campus.SaveConfigFile(cfg, path); err != nil {
		t.Fatal(err)
	}

	loaded, err := campus.LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ACID != "200" {
		t.Fatalf("ACID: got %q, want %q", loaded.ACID, "200")
	}
	if loaded.SSID != "TestWiFi" {
		t.Fatalf("SSID: got %q, want %q", loaded.SSID, "TestWiFi")
	}
	if loaded.Interval != 45*time.Second {
		t.Fatalf("Interval: got %v, want 45s", loaded.Interval)
	}
	if loaded.MaxProbeFails != 5 {
		t.Fatalf("MaxProbeFails: got %d, want 5", loaded.MaxProbeFails)
	}
	if loaded.LogMaxAge != 72*time.Hour {
		t.Fatalf("LogMaxAge: got %v, want 72h", loaded.LogMaxAge)
	}
	// BaseURL matches default, should not be in file
	if loaded.BaseURL != "" {
		t.Fatalf("BaseURL should be empty when matching default, got %q", loaded.BaseURL)
	}
}

func TestLoadConfigFileMissingReturnsError(t *testing.T) {
	_, err := campus.LoadConfigFile(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSaveConfigFileOmitsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Use all defaults except ACID
	cfg := campus.Config{
		BaseURL:        campus.DefaultBaseURL,
		ACID:           "200",
		SSID:           campus.DefaultSSID,
		ProbeURL:       campus.DefaultProbeURL,
		ProbeContains:  campus.DefaultProbeContains,
		UsernameEnv:    campus.DefaultUsernameEnv,
		PasswordEnv:    campus.DefaultPasswordEnv,
		Interval:       campus.DefaultInterval,
		Timeout:        campus.DefaultTimeout,
		ProbeTimeout:   campus.DefaultProbeTimeout,
		StartupTaskName: campus.DefaultStartupTaskName,
		MaxProbeFails:  campus.DefaultMaxProbeFails,
		LogMaxSize:     campus.DefaultLogMaxSize,
		LogMaxBackups:  campus.DefaultLogMaxBackups,
		LogMaxAge:      campus.DefaultLogMaxAge,
	}

	if err := campus.SaveConfigFile(cfg, path); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}

	// Only ac_id should be present
	if v, ok := raw["ac_id"]; !ok || v != "200" {
		t.Fatalf("expected ac_id=200 in config, got %v", raw)
	}
	if _, ok := raw["base_url"]; ok {
		t.Fatal("base_url should not appear when equal to default")
	}
	if _, ok := raw["interval"]; ok {
		t.Fatal("interval should not appear when equal to default")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")

	orig := campus.Config{
		ACID:      "200",
		SSID:      "TestWiFi",
		Interval:  45 * time.Second,
		LogMaxAge: 72 * time.Hour, // differs from default 168h
	}

	if err := campus.SaveConfigFile(orig, path); err != nil {
		t.Fatal(err)
	}

	restored, err := campus.LoadConfigFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if restored.ACID != orig.ACID {
		t.Fatalf("ACID: got %q, want %q", restored.ACID, orig.ACID)
	}
	if restored.SSID != orig.SSID {
		t.Fatalf("SSID: got %q, want %q", restored.SSID, orig.SSID)
	}
	if restored.Interval != orig.Interval {
		t.Fatalf("Interval: got %v, want %v", restored.Interval, orig.Interval)
	}
	if restored.LogMaxAge != orig.LogMaxAge {
		t.Fatalf("LogMaxAge: got %v, want %v", restored.LogMaxAge, orig.LogMaxAge)
	}
}

func TestDefaultConfigFilePath(t *testing.T) {
	path, err := campus.DefaultConfigFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	if filepath.Base(path) != "config.json" {
		t.Fatalf("unexpected filename: %s", filepath.Base(path))
	}
}

func TestDefaultLogFilePath(t *testing.T) {
	path, err := campus.DefaultLogFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "guard.log" {
		t.Fatalf("unexpected filename: %s", filepath.Base(path))
	}
}

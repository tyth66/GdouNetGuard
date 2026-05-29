package campus_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"GdouNetGuard/src"
)

func TestCleanupOldLogsRemovesExpiredRotatedFiles(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "guard.log")

	// Create current log
	if err := os.WriteFile(base, []byte("current"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create rotated files: .1 (old) and .2 (recent)
	if err := os.WriteFile(base+".1", []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(base+".1", oldTime, oldTime)

	if err := os.WriteFile(base+".2", []byte("recent"), 0644); err != nil {
		t.Fatal(err)
	}

	// Cleanup: max age 24 hours
	campus.CleanupOldLogs(base, 24*time.Hour)

	// .1 (48h old) should be gone
	if _, err := os.Stat(base + ".1"); !os.IsNotExist(err) {
		t.Fatal("expected .1 to be removed")
	}
	// .2 (recent) and current should remain
	if _, err := os.Stat(base + ".2"); err != nil {
		t.Fatal("expected .2 to remain")
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatal("expected current log to remain")
	}
}

func TestCleanupOldLogsZeroMaxAgeIsNoop(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "guard.log")
	os.WriteFile(base+".1", []byte("data"), 0644)

	campus.CleanupOldLogs(base, 0)

	if _, err := os.Stat(base + ".1"); err != nil {
		t.Fatal("expected zero maxAge to be a no-op")
	}
}

func TestCleanupOldLogsMissingBaseIsNoop(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "nonexistent.log")
	// Should not panic or error
	campus.CleanupOldLogs(base, 24*time.Hour)
}

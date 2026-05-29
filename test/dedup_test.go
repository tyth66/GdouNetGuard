package campus_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	campus "GdouNetGuard/src"
)

func writeLine(w interface{ Write([]byte) (int, error) }, line string) {
	w.Write([]byte(line + "\n"))
}

func openTestRW(t *testing.T) (*campus.RotatingWriter, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	rw, err := campus.NewRotatingWriter(path, 1<<30, 1)
	if err != nil {
		t.Fatalf("NewRotatingWriter: %v", err)
	}
	t.Cleanup(func() { rw.Close() })
	return rw, path
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	return string(data)
}

func TestDedupWriterCollapsesConsecutiveOnlineLines(t *testing.T) {
	rw, path := openTestRW(t)
	dw := campus.NewDedupWriter(rw)

	writeLine(dw, "2026/05/30 12:00:00 campus auth guard started")
	writeLine(dw, "2026/05/30 12:00:01 online; campus_ip=10.0.0.1 user=alice")
	writeLine(dw, "2026/05/30 12:00:15 online; campus_ip=10.0.0.1 user=alice")
	writeLine(dw, "2026/05/30 12:00:30 online; campus_ip=10.0.0.1 user=alice")
	writeLine(dw, "2026/05/30 12:00:45 online; campus_ip=10.0.0.1 user=alice")
	writeLine(dw, "2026/05/30 12:01:00 campus status unavailable: dial tcp ...")

	output := readLog(t, path)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

	// Should have: startup, first-online, last-online, offline
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (startup, first-online, last-online, offline), got %d:\n%s", len(lines), output)
	}

	if !strings.Contains(lines[1], "12:00:01") {
		t.Errorf("first online line should keep original timestamp, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], "12:00:45") {
		t.Errorf("last online line should have latest timestamp, got: %s", lines[2])
	}
}

func TestDedupWriterBurstsAreSeparated(t *testing.T) {
	rw, path := openTestRW(t)
	dw := campus.NewDedupWriter(rw)

	// First online burst
	writeLine(dw, "2026/05/30 12:00:00 online; campus_ip=10.0.0.1 user=alice")
	writeLine(dw, "2026/05/30 12:00:15 online; campus_ip=10.0.0.1 user=alice")
	writeLine(dw, "2026/05/30 12:00:30 campus status unavailable")
	// Second online burst
	writeLine(dw, "2026/05/30 12:01:00 online; campus_ip=10.0.0.2 user=alice")
	writeLine(dw, "2026/05/30 12:01:15 online; campus_ip=10.0.0.2 user=alice")
	writeLine(dw, "2026/05/30 12:01:30 online; campus_ip=10.0.0.2 user=alice")

	output := readLog(t, path)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d:\n%s", len(lines), output)
	}

	if !strings.Contains(lines[0], "12:00:00") {
		t.Errorf("first line: %s", lines[0])
	}
	if !strings.Contains(lines[1], "12:00:15") {
		t.Errorf("second line: %s", lines[1])
	}
	if !strings.Contains(lines[2], "unavailable") {
		t.Errorf("third line: %s", lines[2])
	}
	if !strings.Contains(lines[3], "12:01:00") {
		t.Errorf("fourth line: %s", lines[3])
	}
	if !strings.Contains(lines[4], "12:01:30") {
		t.Errorf("fifth line: %s", lines[4])
	}
}

func TestDedupWriterSingleOnline(t *testing.T) {
	rw, path := openTestRW(t)
	dw := campus.NewDedupWriter(rw)

	writeLine(dw, "2026/05/30 12:00:00 online; campus_ip=10.0.0.1 user=alice")

	output := readLog(t, path)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d:\n%s", len(lines), output)
	}
}

func TestDedupWriterNonOnlinePassthrough(t *testing.T) {
	rw, path := openTestRW(t)
	dw := campus.NewDedupWriter(rw)

	writeLine(dw, "2026/05/30 12:00:00 campus auth guard started")
	writeLine(dw, "2026/05/30 12:00:05 offline; attempting SRUN login")
	writeLine(dw, "2026/05/30 12:00:10 login accepted")
	writeLine(dw, "2026/05/30 12:00:15 received SIGINT, shutting down")

	output := readLog(t, path)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), output)
	}
}

func TestDedupWriterCloseClosesUnderlying(t *testing.T) {
	rw, _ := openTestRW(t)
	dw := campus.NewDedupWriter(rw)

	if err := dw.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestDedupWriterOnlineLinesHitDiskImmediately(t *testing.T) {
	rw, path := openTestRW(t)
	dw := campus.NewDedupWriter(rw)

	writeLine(dw, "2026/05/30 12:00:01 online; campus_ip=10.0.0.1 user=alice")

	// Should be visible on disk immediately (no buffering delay)
	output := readLog(t, path)
	if !strings.Contains(output, "online;") {
		t.Fatalf("online line should be on disk immediately, got: %q", output)
	}

	// Write a second online line — still visible
	writeLine(dw, "2026/05/30 12:00:15 online; campus_ip=10.0.0.1 user=alice")
	output = readLog(t, path)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 online lines, got %d:\n%s", len(lines), output)
	}

	// Third online line should truncate and replace the second
	writeLine(dw, "2026/05/30 12:00:30 online; campus_ip=10.0.0.1 user=alice")
	output = readLog(t, path)
	lines = strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (first + latest), got %d:\n%s", len(lines), output)
	}
	if !strings.Contains(lines[0], "12:00:01") {
		t.Errorf("first should be 12:00:01, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "12:00:30") {
		t.Errorf("second should be 12:00:30, got: %s", lines[1])
	}
}

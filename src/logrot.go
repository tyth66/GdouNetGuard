package campus

import (
	"fmt"
	"os"
	"sync"
	"time"
)

// RotatingWriter is an io.WriteCloser that automatically rotates the underlying
// log file when it exceeds maxSize bytes. It keeps up to maxBackups rotated
// copies (e.g. guard.log.1, guard.log.2, ...).
// Write operations are safe for concurrent use.
type RotatingWriter struct {
	path       string
	maxSize    int64
	maxBackups int
	file       *os.File
	mu         sync.Mutex
}

// NewRotatingWriter opens the log file at path with append semantics.
// The file is not rotated on construction; the caller should call
// RotateIfNeeded separately to handle the startup rotation case.
func NewRotatingWriter(path string, maxSize int64, maxBackups int) (*RotatingWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", path, err)
	}
	return &RotatingWriter{
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		file:       f,
	}, nil
}

// Write writes p to the log file, rotating first if the write would exceed
// the size limit.
func (w *RotatingWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	info, statErr := w.file.Stat()
	if statErr == nil && info.Size()+int64(len(p)) > w.maxSize {
		w.file.Close()
		rotateBatch(w.path, w.maxBackups)
		f, openErr := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if openErr != nil {
			return 0, fmt.Errorf("reopen log after rotation: %w", openErr)
		}
		w.file = f
	}

	return w.file.Write(p)
}

// Close closes the underlying file.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}

// CurrentSize returns the current file size in bytes, or 0 on error.
func (w *RotatingWriter) CurrentSize() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	info, err := w.file.Stat()
	if err != nil {
		return 0
	}
	return info.Size()
}

// TruncateTo truncates the underlying log file to size bytes.
// It uses os.Truncate on the file path so that append-mode semantics
// are not affected by the open file handle.
func (w *RotatingWriter) TruncateTo(size int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return os.Truncate(w.path, size)
}

// RotateIfNeeded performs a one-shot rotation check. It is intended for
// startup-time use before the RotatingWriter is constructed, so that
// size-triggered rotation happens at startup and runtime.
func RotateIfNeeded(path string, maxSize int64, maxBackups int) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxSize {
		return
	}
	rotateBatch(path, maxBackups)
}

// rotateBatch renames path -> path.1, path.1 -> path.2, etc.,
// and removes the oldest backup.
func rotateBatch(path string, maxBackups int) {
	oldest := path + "." + itoa(maxBackups)
	os.Remove(oldest)
	for i := maxBackups - 1; i >= 1; i-- {
		old := path + "." + itoa(i)
		newPath := path + "." + itoa(i+1)
		os.Rename(old, newPath)
	}
	os.Rename(path, path+".1")
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// CleanupOldLogs removes rotated log files (path.1, path.2, ...) that are
// older than maxAge. The current log file at path is never removed.
func CleanupOldLogs(path string, maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for i := 1; ; i++ {
		rotated := path + "." + itoa(i)
		info, err := os.Stat(rotated)
		if err != nil {
			break // no more rotated files
		}
		if info.ModTime().Before(cutoff) {
			os.Remove(rotated)
		}
	}
}

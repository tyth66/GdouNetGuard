package campus

import (
	"bytes"
	"sync"
)

// maxOnlineLines is the maximum number of consecutive online-status log lines
// to keep before the oldest intermediate line is removed via file truncation.
const maxOnlineLines = 2

// DedupWriter wraps a RotatingWriter and deduplicates consecutive online-status
// log lines at the file level. Every log line is written to disk immediately.
// When a third consecutive "online;" line would be written, the second line is
// removed by truncating the file, keeping only the first and the latest.
type DedupWriter struct {
	rw        *RotatingWriter
	mu        sync.Mutex
	onlineBuf [][]byte // up to maxOnlineLines buffered online-line contents
}

// NewDedupWriter returns a DedupWriter that wraps rw.
func NewDedupWriter(rw *RotatingWriter) *DedupWriter {
	return &DedupWriter{rw: rw}
}

// Write implements io.Writer. Non-online lines flush any buffered state and
// pass through immediately. Online lines are written through and deduplicated
// by truncating the file when the third consecutive online line arrives.
func (dw *DedupWriter) Write(p []byte) (int, error) {
	dw.mu.Lock()
	defer dw.mu.Unlock()

	if !bytes.Contains(p, []byte("online;")) {
		dw.onlineBuf = dw.onlineBuf[:0]
		return dw.rw.Write(p)
	}
	return dw.writeOnline(p)
}

// Close implements io.Closer.
func (dw *DedupWriter) Close() error {
	return dw.rw.Close()
}

// writeOnline writes an online line through to disk, deduplicating when
// necessary. Caller must hold dw.mu.
func (dw *DedupWriter) writeOnline(p []byte) (int, error) {
	line := make([]byte, len(p))
	copy(line, p)

	if len(dw.onlineBuf) < maxOnlineLines {
		dw.onlineBuf = append(dw.onlineBuf, line)
		return dw.rw.Write(p)
	}

	// We already have maxOnlineLines on disk (buf[0] and buf[1]).
	// Truncate the file to remove buf[1], then write the new line
	// as the replacement.
	curSize := dw.rw.CurrentSize()
	targetSize := curSize - int64(len(dw.onlineBuf[1]))
	if targetSize < 0 {
		targetSize = 0
	}
	if err := dw.rw.TruncateTo(targetSize); err != nil {
		return 0, err
	}
	dw.onlineBuf[1] = line
	return dw.rw.Write(p)
}

package tasks

import (
	"io"
	"os"
	"sync"
)

// LogBuffer is an append-only growing in-memory buffer that wakes blocked
// readers when new bytes arrive. Closed once the task reaches a terminal phase.
type LogBuffer struct {
	mu     sync.Mutex
	cond   *sync.Cond
	data   []byte
	closed bool
	file   *os.File
}

func NewLogBuffer(filePath string) *LogBuffer {
	b := &LogBuffer{}
	b.cond = sync.NewCond(&b.mu)
	if filePath != "" {
		if f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
			b.file = f
		}
	}
	return b
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.data = append(b.data, p...)
	if b.file != nil {
		_, _ = b.file.Write(p)
	}
	b.cond.Broadcast()
	b.mu.Unlock()
	return len(p), nil
}

func (b *LogBuffer) Close() {
	b.mu.Lock()
	b.closed = true
	if b.file != nil {
		_ = b.file.Close()
		b.file = nil
	}
	b.cond.Broadcast()
	b.mu.Unlock()
}

func (b *LogBuffer) Snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.data))
	copy(out, b.data)
	return out
}

// Reader returns an io.Reader that yields all bytes from offset 0. If follow
// is true, blocks on Read until new bytes arrive; returns io.EOF once the
// buffer is closed.
func (b *LogBuffer) Reader(follow bool) io.Reader {
	return &logReader{buf: b, follow: follow}
}

type logReader struct {
	buf    *LogBuffer
	offset int
	follow bool
}

func (r *logReader) Read(p []byte) (int, error) {
	r.buf.mu.Lock()
	defer r.buf.mu.Unlock()
	for r.offset >= len(r.buf.data) {
		if r.buf.closed || !r.follow {
			return 0, io.EOF
		}
		r.buf.cond.Wait()
	}
	n := copy(p, r.buf.data[r.offset:])
	r.offset += n
	return n, nil
}

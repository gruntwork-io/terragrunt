package util

import (
	"io"
	"sync"
)

// SyncWriter wraps an io.Writer with a mutex to make it safe for concurrent use.
// This is necessary when multiple goroutines write to the same writer, such as
// when running terraform commands in parallel during "run --all" operations.
type SyncWriter struct {
	w  io.Writer
	mu sync.Mutex
}

// NewSyncWriter returns a new SyncWriter that wraps the given writer.
func NewSyncWriter(w io.Writer) *SyncWriter {
	return &SyncWriter{w: w}
}

// Write implements the io.Writer interface with synchronization.
func (sw *SyncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	return sw.w.Write(p)
}

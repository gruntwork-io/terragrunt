package config

import (
	"maps"
	"slices"
	"sync"
)

// FilesRead is a concurrency-safe set of file paths that have been read while
// parsing a configuration. A nil receiver is treated as a no-op for mutations
// and returns zero values from accessors, so call sites that don't need to
// track reads can pass nil.
type FilesRead struct {
	seen map[string]struct{}
	mu   sync.RWMutex
}

// NewFilesRead returns an empty FilesRead ready for concurrent use.
func NewFilesRead() *FilesRead {
	return &FilesRead{}
}

// Add records path as read. Duplicate paths are ignored.
func (f *FilesRead) Add(path string) {
	if f == nil {
		return
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.seen == nil {
		f.seen = make(map[string]struct{})
	}

	f.seen[path] = struct{}{}
}

// Paths returns the recorded paths in lexical order.
func (f *FilesRead) Paths() []string {
	if f == nil {
		return nil
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	return slices.Sorted(maps.Keys(f.seen))
}

// Len returns the number of recorded paths.
func (f *FilesRead) Len() int {
	if f == nil {
		return 0
	}

	f.mu.RLock()
	defer f.mu.RUnlock()

	return len(f.seen)
}

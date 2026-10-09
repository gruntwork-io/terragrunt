package config

import (
	"maps"
	"slices"
	"sync"
)

// FilesRead is a concurrency-safe set of file paths that have been read while
// parsing a configuration. A nil receiver is treated as a no-op for mutations
// and returns zero values from accessors, and nil is what a parsing context
// carries until a caller asks for tracking, so every writer degrades to doing
// nothing rather than the caller having to check first.
type FilesRead struct {
	seen     map[string]struct{}
	seenDirs map[string]struct{}
	mu       sync.Mutex
}

// NewFilesRead returns an empty FilesRead ready for concurrent use.
func NewFilesRead() *FilesRead {
	return &FilesRead{}
}

// Tracking reports whether reads are being recorded. Work whose only product is
// a recorded read can be skipped when it returns false.
func (f *FilesRead) Tracking() bool {
	return f != nil
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

	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Sorted(maps.Keys(f.seen))
}

// Len returns the number of recorded paths.
func (f *FilesRead) Len() int {
	if f == nil {
		return 0
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.seen)
}

// MarkDirIfNew records dir as a walked module directory and returns true if dir
// had not been visited before, false if it was already recorded. The check and
// mark are a single atomic operation, so concurrent callers for the same dir
// produce exactly one true result.
func (f *FilesRead) MarkDirIfNew(dir string) bool {
	if f == nil {
		return true
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	if f.seenDirs == nil {
		f.seenDirs = make(map[string]struct{})
	}

	if _, ok := f.seenDirs[dir]; ok {
		return false
	}

	f.seenDirs[dir] = struct{}{}

	return true
}

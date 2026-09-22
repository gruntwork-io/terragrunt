package vfs

import (
	"path/filepath"
	"sync"
)

// PathResolver memoizes [ResolveForCompare] against one filesystem, so each
// distinct path is resolved once for the resolver's lifetime. A PathResolver
// is safe for concurrent use.
//
// A symlink created, removed, or retargeted after a path is resolved is not
// seen, so a resolver should live no longer than one pass over a tree whose
// symlinks do not change.
type PathResolver struct {
	fsys     FS
	resolved map[string]string
	mu       sync.Mutex
}

// NewPathResolver returns a PathResolver that resolves paths against fsys.
func NewPathResolver(fsys FS) *PathResolver {
	return &PathResolver{fsys: fsys, resolved: make(map[string]string)}
}

// Resolve returns [ResolveForCompare] of path, from memory when path, once
// cleaned, was resolved before.
func (r *PathResolver) Resolve(path string) string {
	path = filepath.Clean(path)

	r.mu.Lock()
	resolved, ok := r.resolved[path]
	r.mu.Unlock()

	if ok {
		return resolved
	}

	resolved = ResolveForCompare(r.fsys, path)

	r.mu.Lock()
	r.resolved[path] = resolved
	r.mu.Unlock()

	return resolved
}

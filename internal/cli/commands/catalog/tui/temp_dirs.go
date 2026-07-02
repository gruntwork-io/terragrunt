package tui

import (
	"sort"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// TempDirTracker records catalog clone roots that must survive until the TUI
// exits because unit and stack actions read files from the cloned repository.
type TempDirTracker struct {
	fsys vfs.FS
	dirs map[string]struct{}
	mu   sync.Mutex
}

// NewTempDirTracker creates a tracker for temporary catalog clone roots.
func NewTempDirTracker(fsys vfs.FS) *TempDirTracker {
	return &TempDirTracker{
		fsys: fsys,
		dirs: make(map[string]struct{}),
	}
}

// Track records path for cleanup when the catalog session exits.
func (t *TempDirTracker) Track(path string) {
	if t == nil || path == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.dirs[path] = struct{}{}
}

// Cleanup removes tracked paths. Cleanup errors are logged but do not change
// the catalog command result because they happen after the user action ended.
func (t *TempDirTracker) Cleanup(l log.Logger) {
	if t == nil {
		return
	}

	for _, dir := range t.dirsSnapshot() {
		if err := t.fsys.RemoveAll(dir); err != nil {
			l.Warnf("Failed to remove catalog temporary directory %q: %v", dir, err)
		}
	}
}

func (t *TempDirTracker) dirsSnapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	dirs := make([]string, 0, len(t.dirs))
	for dir := range t.dirs {
		dirs = append(dirs, dir)
	}

	clear(t.dirs)
	sort.Strings(dirs)

	return dirs
}

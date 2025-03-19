package cas

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Store manages the store directory and locks to prevent concurrent writes
type Store struct {
	locks   map[string]*sync.Mutex
	path    string
	mapLock sync.Mutex
}

// NewStore creates a new Store instance.
func NewStore(path string) *Store {
	return &Store{
		path:  path,
		locks: make(map[string]*sync.Mutex),
	}
}

// Path returns the current store path
func (s *Store) Path() string {
	return s.path
}

// NeedsWrite checks if a given hash needs to be stored
func (s *Store) NeedsWrite(hash string, cloneStart time.Time) bool {
	partitionDir := filepath.Join(s.path, hash[:2])
	path := filepath.Join(partitionDir, hash)

	return !s.hasContent(path) && !s.writeInProgress(path, cloneStart)
}

// HasContent checks if a given hash exists in the store
func (s *Store) hasContent(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

// writeInProgress checks if a write is in progress for a given hash
func (s *Store) writeInProgress(path string, cloneStart time.Time) bool {
	path += ".tmp"

	stat, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}

	// To avoid deadlocks, assume that a write is in progress
	// only if the file exists and was modified
	// after the start of cloning.
	modifiedTime := stat.ModTime()

	return modifiedTime.After(cloneStart)
}

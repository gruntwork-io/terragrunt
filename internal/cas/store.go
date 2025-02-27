package cas

import (
	"os"
	"path/filepath"
	"sync"
)

// Store manages the store directory and locks to prevent concurrent writes
type Store struct {
	path    string
	locks   map[string]*sync.Mutex
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

// HasContent checks if a given hash exists in the store
func (s *Store) HasContent(hash string) bool {
	// Use partitioned path: first two characters of hash as subdirectory
	partitionDir := filepath.Join(s.path, hash[:2])
	path := filepath.Join(partitionDir, hash)
	_, err := os.Stat(path)

	return err == nil && !os.IsNotExist(err)
}

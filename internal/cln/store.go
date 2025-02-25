package cln

import (
	"os"
	"path/filepath"
	"sync"
)

const (
	// storePathPerm defines the default permission (0755) for the store directory
	// equivalent to user:rwx group:rx others:rx
	storePathPerm = 0755
)

// Store manages the path where cloned repositories are stored
type Store struct {
	path         string
	contentCache sync.Map
	mu           sync.RWMutex
}

// NewStore creates a new Store instance. If path is empty, it will use
// $HOME/.cln-store
func NewStore(path string) (*Store, error) {
	if path != "" {
		return &Store{path: path}, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, &WrappedError{
			Op:  "get_home_dir",
			Err: ErrHomeDir,
		}
	}

	storePath := filepath.Join(home, ".cln-store")
	if err := os.MkdirAll(storePath, storePathPerm); err != nil {
		return nil, &WrappedError{
			Op:   "create_store_dir",
			Path: storePath,
			Err:  ErrCreateDir,
		}
	}

	return &Store{path: storePath}, nil
}

// Path returns the current store path
func (s *Store) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

// HasContent checks if a given hash exists in the store
func (s *Store) HasContent(hash string) bool {
	// Check cache first
	if _, ok := s.contentCache.Load(hash); ok {
		return true
	}

	path := filepath.Join(s.path, hash)
	_, err := os.Stat(path)
	exists := err == nil

	if exists {
		s.contentCache.Store(hash, struct{}{})
	}

	return exists
}

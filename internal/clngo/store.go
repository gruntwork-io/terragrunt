package clngo

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
	path string
	// Add cache for frequently accessed content
	contentCache sync.Map
	mu           sync.RWMutex // Add mutex for file operations
}

// NewStore creates a new Store instance. If path is empty, it will use
// $HOME/.cache/.cln-store
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

	storePath := filepath.Join(home, ".cache", ".cln-store")
	if err := os.MkdirAll(storePath, storePathPerm); err != nil {
		return nil, &WrappedError{
			Op:   "create_store_dir",
			Path: storePath,
			Err:  ErrCreateDir,
		}
	}

	// Ensure refs directory exists
	refsPath := filepath.Join(storePath, "refs")
	if err := os.MkdirAll(refsPath, storePathPerm); err != nil {
		return nil, &WrappedError{
			Op:   "create_refs_dir",
			Path: refsPath,
			Err:  ErrCreateDir,
		}
	}

	return &Store{path: storePath}, nil
}

// Path returns the current store path
func (s *Store) Path() string {
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

// HasReference checks if a complete reference exists in the store
func (s *Store) HasReference(hash string) bool {
	refPath := filepath.Join(s.path, "refs", hash)
	_, err := os.Stat(refPath)
	return err == nil
}

// StoreReference marks a git reference as completely stored
func (s *Store) StoreReference(hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	refPath := filepath.Join(s.path, "refs", hash)

	// Check if reference already exists to avoid race conditions
	if s.HasReference(hash) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(refPath), DefaultDirPerms); err != nil {
		return wrapError("create_refs_dir", filepath.Dir(refPath), ErrCreateDir)
	}

	// Use atomic write operation
	tempFile := refPath + ".tmp"
	if err := os.WriteFile(tempFile, []byte{}, StoredFilePerms); err != nil {
		return wrapError("write_temp_reference", tempFile, ErrWriteToStore)
	}

	if err := os.Rename(tempFile, refPath); err != nil {
		os.Remove(tempFile) // Clean up temp file
		return wrapError("store_reference", refPath, ErrWriteToStore)
	}

	return nil
}

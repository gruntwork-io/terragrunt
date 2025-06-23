package cas

import (
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// Store manages the store directory and filesystem locks to prevent concurrent writes
type Store struct {
	path string
}

// NewStore creates a new Store instance.
func NewStore(path string) *Store {
	return &Store{
		path: path,
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

// AcquireLock acquires a filesystem lock for the given hash
// Returns the flock instance that should be unlocked when done
func (s *Store) AcquireLock(hash string) (*flock.Flock, error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	lockPath := filepath.Join(partitionDir, hash+".lock")

	// Ensure the partition directory exists
	if err := os.MkdirAll(partitionDir, 0755); err != nil {
		return nil, err
	}

	lock := flock.New(lockPath)
	if err := lock.Lock(); err != nil {
		return nil, err
	}

	return lock, nil
}

// TryAcquireLock attempts to acquire a filesystem lock for the given hash without blocking
// Returns the flock instance and true if successful, nil and false if the lock is already held
func (s *Store) TryAcquireLock(hash string) (*flock.Flock, bool, error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	lockPath := filepath.Join(partitionDir, hash+".lock")

	// Ensure the partition directory exists
	if err := os.MkdirAll(partitionDir, 0755); err != nil {
		return nil, false, err
	}

	lock := flock.New(lockPath)
	acquired, err := lock.TryLock()
	if err != nil {
		return nil, false, err
	}

	if !acquired {
		return nil, false, nil
	}

	return lock, true, nil
}

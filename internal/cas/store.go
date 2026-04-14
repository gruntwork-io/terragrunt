package cas

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Store manages the store directory and filesystem locks to prevent concurrent writes
type Store struct {
	fs   vfs.FS
	path string
}

// NewStore creates a new Store instance with the OS filesystem.
func NewStore(path string) *Store {
	return &Store{
		path: path,
		fs:   vfs.NewOSFS(),
	}
}

// WithFS sets the filesystem for file operations and returns the Store for method chaining.
func (s *Store) WithFS(fs vfs.FS) *Store {
	s.fs = fs
	return s
}

// FS returns the configured filesystem.
func (s *Store) FS() vfs.FS {
	return s.fs
}

// Path returns the current store path
func (s *Store) Path() string {
	return s.path
}

// NeedsWrite checks if a given hash needs to be stored
func (s *Store) NeedsWrite(hash string) bool {
	partitionDir := filepath.Join(s.path, hash[:2])
	path := filepath.Join(partitionDir, hash)

	return !s.hasContent(path)
}

// HasContent checks if a given hash exists in the store
func (s *Store) hasContent(path string) bool {
	_, err := s.fs.Stat(path)

	return err == nil
}

// AcquireLock acquires a filesystem lock for the given hash
// Returns the lock that should be unlocked when done
func (s *Store) AcquireLock(hash string) (vfs.Unlocker, error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	lockPath := filepath.Join(partitionDir, hash+".lock")

	// Ensure the partition directory exists
	if err := s.fs.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return nil, err
	}

	return vfs.Lock(s.fs, lockPath)
}

// TryAcquireLock attempts to acquire a filesystem lock for the given hash without blocking
// Returns the lock and true if successful, nil and false if the lock is already held
func (s *Store) TryAcquireLock(hash string) (vfs.Unlocker, bool, error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	lockPath := filepath.Join(partitionDir, hash+".lock")

	// Ensure the partition directory exists
	if err := s.fs.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return nil, false, err
	}

	return vfs.TryLock(s.fs, lockPath)
}

// EnsureWithWait tries to acquire a lock for the given hash, and if another process
// is writing the same content, waits for it to complete instead of doing redundant work.
// This is an optimization for read operations that avoids duplicate writes.
//
// Returns:
// - needsWrite: true if content doesn't exist and caller should write it
// - lock: the acquired lock (nil if needsWrite is false)
// - error: any error that occurred
func (s *Store) EnsureWithWait(hash string) (needsWrite bool, lock vfs.Unlocker, err error) {
	// Fast path: check if content already exists
	partitionDir := filepath.Join(s.path, hash[:2])
	path := filepath.Join(partitionDir, hash)

	if s.hasContent(path) {
		return false, nil, nil
	}

	// Try to acquire lock without blocking
	tryLock, acquired, err := s.TryAcquireLock(hash)
	if err != nil {
		return false, nil, err
	}

	if acquired {
		// We got the lock immediately, check if we still need to write
		// (another process might have completed while we were trying)
		if !s.NeedsWrite(hash) {
			// Content appeared while we were acquiring lock, no write needed
			if err = tryLock.Unlock(); err != nil {
				return false, nil, err
			}

			return false, nil, nil
		}
		// We have the lock and content doesn't exist, caller should write
		return true, tryLock, nil
	}

	// Lock is held by another process, wait for it to complete
	waitLock, err := s.AcquireLock(hash)
	if err != nil {
		return false, nil, err
	}

	// Now we have the lock, check if the other process wrote the content
	if !s.NeedsWrite(hash) {
		// Content was written by the other process, no write needed
		if err := waitLock.Unlock(); err != nil {
			return false, nil, err
		}

		return false, nil, nil
	}

	// Content still doesn't exist, caller should write it
	return true, waitLock, nil
}

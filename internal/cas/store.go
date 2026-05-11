package cas

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Store manages the store directory and filesystem locks to prevent concurrent writes.
type Store struct {
	path string
}

// NewStore creates a new Store rooted at path.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the current store path.
func (s *Store) Path() string {
	return s.path
}

// NeedsWrite checks if a given hash needs to be stored.
func (s *Store) NeedsWrite(v Venv, hash string) bool {
	partitionDir := filepath.Join(s.path, hash[:2])
	path := filepath.Join(partitionDir, hash)

	return !s.hasContent(v, path)
}

// AcquireLock acquires a filesystem lock for the given hash.
// Returns the lock that should be unlocked when done.
func (s *Store) AcquireLock(v Venv, hash string) (vfs.Unlocker, error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	lockPath := filepath.Join(partitionDir, hash+".lock")

	if err := v.FS.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return nil, err
	}

	return vfs.Lock(v.FS, lockPath)
}

// TryAcquireLock attempts to acquire a filesystem lock for the given hash without blocking.
// Returns the lock and true if successful, nil and false if the lock is already held.
func (s *Store) TryAcquireLock(v Venv, hash string) (vfs.Unlocker, bool, error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	lockPath := filepath.Join(partitionDir, hash+".lock")

	if err := v.FS.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return nil, false, err
	}

	return vfs.TryLock(v.FS, lockPath)
}

// EnsureWithWait tries to acquire a lock for the given hash, and if another process
// is writing the same content, waits for it to complete instead of doing redundant work.
// This is an optimization for read operations that avoids duplicate writes.
//
// Returns:
//   - needsWrite: true if content doesn't exist and caller should write it
//   - lock: the acquired lock (nil if needsWrite is false)
//   - error: any error that occurred
func (s *Store) EnsureWithWait(v Venv, hash string) (needsWrite bool, lock vfs.Unlocker, err error) {
	partitionDir := filepath.Join(s.path, hash[:2])
	path := filepath.Join(partitionDir, hash)

	if s.hasContent(v, path) {
		return false, nil, nil
	}

	tryLock, acquired, err := s.TryAcquireLock(v, hash)
	if err != nil {
		return false, nil, err
	}

	if acquired {
		if !s.NeedsWrite(v, hash) {
			if err = tryLock.Unlock(); err != nil {
				return false, nil, err
			}

			return false, nil, nil
		}

		return true, tryLock, nil
	}

	waitLock, err := s.AcquireLock(v, hash)
	if err != nil {
		return false, nil, err
	}

	if !s.NeedsWrite(v, hash) {
		if err := waitLock.Unlock(); err != nil {
			return false, nil, err
		}

		return false, nil, nil
	}

	return true, waitLock, nil
}

func (s *Store) hasContent(v Venv, path string) bool {
	_, err := v.FS.Stat(path)

	return err == nil
}

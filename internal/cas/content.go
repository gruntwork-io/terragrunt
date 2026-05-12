package cas

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// DefaultDirPerms represents standard directory permissions (rwxr-xr-x)
	DefaultDirPerms = os.FileMode(0755)
	// StoredFilePerms represents read-only file permissions (r--r--r--)
	StoredFilePerms = os.FileMode(0444)
	// RegularFilePerms represents standard file permissions (rw-r--r--)
	RegularFilePerms = os.FileMode(0644)
	// WindowsOS is the name of the Windows operating system
	WindowsOS = "windows"
)

// Content manages git object storage and linking
type Content struct {
	store *Store
	fs    vfs.FS
}

// NewContent creates a new Content instance
func NewContent(store *Store) *Content {
	return &Content{
		store: store,
		fs:    store.FS(),
	}
}

// Link creates a hard link from the store to the target path
func (c *Content) Link(ctx context.Context, hash, targetPath string) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "cas_link", map[string]any{
		"hash": hash,
		"path": targetPath,
	}, func(childCtx context.Context) error {
		sourcePath := c.getPath(hash)

		// Try to create hard link directly (most efficient path)
		if err := vfs.Link(c.fs, sourcePath, targetPath); err != nil {
			// Check if it's because target already exists
			if os.IsExist(err) {
				// File already exists, which is fine
				return nil
			}

			// If hard link fails for other reasons, try to copy the file
			data, readErr := vfs.ReadFile(c.fs, sourcePath)
			if readErr != nil {
				return &WrappedError{
					Op:   "read_source",
					Path: sourcePath,
					Err:  ErrReadFile,
				}
			}

			// Write to temporary file first
			tempPath := targetPath + ".tmp"
			if err := vfs.WriteFile(c.fs, tempPath, data, RegularFilePerms); err != nil {
				return &WrappedError{
					Op:   "write_target",
					Path: tempPath,
					Err:  err,
				}
			}

			// Atomic rename to final path
			if err := c.fs.Rename(tempPath, targetPath); err != nil {
				return &WrappedError{
					Op:   "rename_target",
					Path: tempPath,
					Err:  err,
				}
			}
		}

		return nil
	})
}

// Store stores a single content item. This is typically used for trees,
// As blobs are written directly from git cat-file stdout.
func (c *Content) Store(l log.Logger, hash string, data []byte) error {
	lock, err := c.store.AcquireLock(hash)
	if err != nil {
		return fmt.Errorf("acquire lock for %s: %w", hash, err)
	}

	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			l.Warnf("failed to unlock filesystem lock for hash %s: %v", hash, unlockErr)
		}
	}()

	if err = c.fs.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return fmt.Errorf("create store dir %s: %w", c.store.Path(), ErrCreateDir)
	}

	// Ensure partition directory exists
	partitionDir := c.getPartition(hash)
	if err = c.fs.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	return c.writeContentToFile(l, hash, data)
}

// Ensure ensures that a content item exists in the store
func (c *Content) Ensure(l log.Logger, hash string, data []byte) error {
	path := c.getPath(hash)
	if c.store.hasContent(path) {
		return nil
	}

	return c.Store(l, hash, data)
}

// EnsureWithWait ensures that a content item exists in the store, with optimization
// to wait for concurrent writes instead of doing redundant work
func (c *Content) EnsureWithWait(l log.Logger, hash string, data []byte) error {
	needsWrite, lock, err := c.store.EnsureWithWait(hash)
	if err != nil {
		return fmt.Errorf("ensure content for %s: %w", hash, err)
	}

	// If content already exists or was written by another process, we're done
	if !needsWrite {
		return nil
	}

	// We have the lock and need to write the content
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			l.Warnf("failed to unlock filesystem lock for hash %s: %v", hash, unlockErr)
		}
	}()

	if err = c.fs.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return fmt.Errorf("create store dir %s: %w", c.store.Path(), ErrCreateDir)
	}

	// Ensure partition directory exists
	partitionDir := c.getPartition(hash)
	if err = c.fs.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	return c.writeContentToFile(l, hash, data)
}

// EnsureCopy ensures that a content item exists in the store by copying from a file
func (c *Content) EnsureCopy(l log.Logger, hash, src string) (err error) {
	path := c.getPath(hash)
	if c.store.hasContent(path) {
		return nil
	}

	lock, err := c.store.AcquireLock(hash)
	if err != nil {
		return fmt.Errorf("acquire lock for %s: %w", hash, err)
	}

	defer func() {
		err = errors.Join(err, lock.Unlock())
	}()

	// Ensure partition directory exists
	partitionDir := c.getPartition(hash)
	if err = c.fs.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	f, err := c.fs.Create(path)
	if err != nil {
		return fmt.Errorf("create file %s: %w", path, err)
	}

	defer func() {
		err = errors.Join(err, f.Close())
	}()

	r, err := c.fs.Open(src)
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}

	defer func() {
		err = errors.Join(err, r.Close())
	}()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("copy from %s: %w", src, err)
	}

	return nil
}

// GetTmpHandle returns a file handle to a temporary file where content will be stored.
func (c *Content) GetTmpHandle(hash string) (vfs.File, error) {
	partitionDir := c.getPartition(hash)
	if err := c.fs.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	path := c.getPath(hash)
	tempPath := path + ".tmp"

	f, err := c.fs.Create(tempPath)
	if err != nil {
		return nil, fmt.Errorf("create temp file %s: %w", tempPath, err)
	}

	return f, err
}

// Read retrieves content from the store by hash
func (c *Content) Read(hash string) ([]byte, error) {
	path := c.getPath(hash)
	return vfs.ReadFile(c.fs, path)
}

// writeContentToFile writes data to a temporary file,
// sets appropriate permissions, and performs an atomic rename.
func (c *Content) writeContentToFile(l log.Logger, hash string, data []byte) error {
	path := c.getPath(hash)
	tempPath := path + ".tmp"

	f, err := c.fs.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, RegularFilePerms)
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", tempPath, err)
	}

	buf := bufio.NewWriter(f)

	if _, err := buf.Write(data); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			l.Warnf("failed to close temp file %s: %v", tempPath, closeErr)
		}

		if removeErr := c.fs.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("write to %s: %w", tempPath, err)
	}

	if err := buf.Flush(); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			l.Warnf("failed to close temp file %s: %v", tempPath, closeErr)
		}

		if removeErr := c.fs.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("flush %s: %w", tempPath, err)
	}

	if err := f.Close(); err != nil {
		if removeErr := c.fs.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("close %s: %w", tempPath, err)
	}

	// Set read-only permissions on the temporary file
	if err := c.fs.Chmod(tempPath, StoredFilePerms); err != nil {
		if removeErr := c.fs.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("chmod temp %s: %w", tempPath, err)
	}

	// For Windows, handle readonly attributes specifically
	if runtime.GOOS == WindowsOS {
		// Check if a destination file exists and is read-only
		if _, err := c.fs.Stat(path); err == nil {
			// File exists, make it writable before rename operation
			if err := c.fs.Chmod(path, RegularFilePerms); err != nil {
				l.Warnf("failed to make destination file writable %s: %v", path, err)
			}
		}
	}

	// Atomic rename
	if err := c.fs.Rename(tempPath, path); err != nil {
		if removeErr := c.fs.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("finalize %s: %w", path, err)
	}

	// For Windows, we need to set the permissions again after rename
	if runtime.GOOS == WindowsOS {
		// Ensure the file has read-only permissions after rename
		if err := c.fs.Chmod(path, StoredFilePerms); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}
	}

	return nil
}

// getPartition returns the partition path for a given hash
func (c *Content) getPartition(hash string) string {
	return filepath.Join(c.store.Path(), hash[:2])
}

// getPath returns the full path for a given hash
func (c *Content) getPath(hash string) string {
	return filepath.Join(c.getPartition(hash), hash)
}

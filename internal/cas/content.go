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
	// DefaultDirPerms represents standard directory permissions (rwxr-xr-x).
	DefaultDirPerms = os.FileMode(0755)
	// StoredFilePerms represents read-only file permissions (r--r--r--).
	StoredFilePerms = os.FileMode(0444)
	// RegularFilePerms represents standard file permissions (rw-r--r--).
	RegularFilePerms = os.FileMode(0644)
	// WriteBitMask covers all owner/group/other write bits.
	WriteBitMask = os.FileMode(0o222)
	// WindowsOS is the name of the Windows operating system.
	WindowsOS = "windows"
)

// Content manages git object storage and linking.
type Content struct {
	store *Store
}

// NewContent creates a new Content instance bound to store.
func NewContent(store *Store) *Content {
	return &Content{store: store}
}

// LinkOption configures a single Content.Link call.
type LinkOption func(*linkOpts)

type linkOpts struct {
	forceCopy bool
}

// WithLinkForceCopy makes Link copy the file from the store into the target
// path instead of creating a hard link, so the destination is safe to mutate
// without affecting the shared store.
func WithLinkForceCopy() LinkOption {
	return func(o *linkOpts) { o.forceCopy = true }
}

// Link materializes a stored blob at targetPath under gitPerm.
//
// The default path hardlinks the stored blob with its write bits stripped, so
// the destination cannot be edited back into the shared store. The fallback
// copy path applies when stored perms don't match the request (rare cross-mode
// collision) or when [WithLinkForceCopy] is in effect, so callers can edit
// the working tree freely.
func (c *Content) Link(
	ctx context.Context,
	v Venv,
	hash, targetPath string,
	gitPerm os.FileMode,
	opts ...LinkOption,
) error {
	var o linkOpts
	for _, opt := range opts {
		opt(&o)
	}

	desired := gitPerm.Perm()
	if !o.forceCopy {
		desired &^= WriteBitMask
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "cas_link", map[string]any{
		"hash":       hash,
		"path":       targetPath,
		"force_copy": o.forceCopy,
		"perm":       uint32(desired),
	}, func(childCtx context.Context) error {
		sourcePath := c.getPath(hash)

		// Hardlink when the stored blob's perms already match what the caller
		// wants. Otherwise we must produce a fresh inode so a chmod cannot
		// leak back into the shared store and so the destination carries the
		// requested mode.
		if !o.forceCopy {
			if info, statErr := v.FS.Stat(sourcePath); statErr == nil && info.Mode().Perm() == desired {
				if err := vfs.Link(v.FS, sourcePath, targetPath); err == nil {
					return nil
				}
				// Fall through to copy on link failure. An existing
				// targetPath is handled by the temp-file+rename below,
				// which overwrites stale bytes atomically.
			}
		}

		data, readErr := vfs.ReadFile(v.FS, sourcePath)
		if readErr != nil {
			return &WrappedError{
				Op:   "read_source",
				Path: sourcePath,
				Err:  ErrReadFile,
			}
		}

		tempPath := targetPath + ".tmp"
		if err := vfs.WriteFile(v.FS, tempPath, data, desired); err != nil {
			return &WrappedError{
				Op:   "write_target",
				Path: tempPath,
				Err:  err,
			}
		}

		// Reapply perms after write to override any umask masking.
		if err := v.FS.Chmod(tempPath, desired); err != nil {
			return &WrappedError{
				Op:   "chmod_target",
				Path: tempPath,
				Err:  err,
			}
		}

		if err := v.FS.Rename(tempPath, targetPath); err != nil {
			return &WrappedError{
				Op:   "rename_target",
				Path: tempPath,
				Err:  err,
			}
		}

		return nil
	})
}

// Store stores a single content item. This is typically used for trees,
// as blobs are written directly from git cat-file stdout.
func (c *Content) Store(l log.Logger, v Venv, hash string, data []byte) error {
	lock, err := c.store.AcquireLock(v, hash)
	if err != nil {
		return fmt.Errorf("acquire lock for %s: %w", hash, err)
	}

	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			l.Warnf("failed to unlock filesystem lock for hash %s: %v", hash, unlockErr)
		}
	}()

	if err = v.FS.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return fmt.Errorf("create store dir %s: %w", c.store.Path(), ErrCreateDir)
	}

	partitionDir := c.getPartition(hash)
	if err = v.FS.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	return c.writeContentToFile(l, v, hash, data)
}

// Ensure ensures that a content item exists in the store.
func (c *Content) Ensure(l log.Logger, v Venv, hash string, data []byte) error {
	path := c.getPath(hash)
	if c.store.hasContent(v, path) {
		return nil
	}

	return c.Store(l, v, hash, data)
}

// EnsureWithWait ensures that a content item exists in the store, with optimization
// to wait for concurrent writes instead of doing redundant work.
func (c *Content) EnsureWithWait(l log.Logger, v Venv, hash string, data []byte) error {
	needsWrite, lock, err := c.store.EnsureWithWait(v, hash)
	if err != nil {
		return fmt.Errorf("ensure content for %s: %w", hash, err)
	}

	if !needsWrite {
		return nil
	}

	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			l.Warnf("failed to unlock filesystem lock for hash %s: %v", hash, unlockErr)
		}
	}()

	if err = v.FS.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return fmt.Errorf("create store dir %s: %w", c.store.Path(), ErrCreateDir)
	}

	partitionDir := c.getPartition(hash)
	if err = v.FS.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	return c.writeContentToFile(l, v, hash, data)
}

// EnsureCopy ensures that a content item exists in the store by copying from a file.
// The stored blob is chmodded to the source file's perms with the write bits cleared,
// so the default-link path can hardlink the blob directly without losing its
// executable-ness or risking writes back into the shared store.
func (c *Content) EnsureCopy(l log.Logger, v Venv, hash, src string) (err error) {
	path := c.getPath(hash)
	if c.store.hasContent(v, path) {
		return nil
	}

	srcInfo, err := v.FS.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %s: %w", src, err)
	}

	lock, err := c.store.AcquireLock(v, hash)
	if err != nil {
		return fmt.Errorf("acquire lock for %s: %w", hash, err)
	}

	defer func() {
		err = errors.Join(err, lock.Unlock())
	}()

	// Re-check under the lock: another worker may have raced ahead and stored
	// a read-only blob between the lock-free hasContent check and AcquireLock.
	// Without this guard, Create below would fail with EACCES on the existing
	// 0o444 file.
	if c.store.hasContent(v, path) {
		return nil
	}

	partitionDir := c.getPartition(hash)
	if err = v.FS.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	// Write through a tempPath so a crash mid-copy cannot leave a
	// half-written blob at the final hash-addressed path. The rename
	// is the publish step.
	tempPath := path + ".tmp"

	f, err := v.FS.Create(tempPath)
	if err != nil {
		return fmt.Errorf("create file %s: %w", tempPath, err)
	}

	// renamed flips after the publish step so the deferred cleanup
	// removes a stale tempPath only on the error path.
	renamed := false

	defer func() {
		if renamed {
			return
		}

		if rmErr := v.FS.Remove(tempPath); rmErr != nil && !os.IsNotExist(rmErr) {
			err = errors.Join(err, rmErr)
		}
	}()

	r, err := v.FS.Open(src)
	if err != nil {
		err = errors.Join(err, f.Close())
		return fmt.Errorf("open source %s: %w", src, err)
	}

	defer func() {
		err = errors.Join(err, r.Close())
	}()

	if _, err := io.Copy(f, r); err != nil {
		closeErr := f.Close()
		return fmt.Errorf("copy from %s: %w", src, errors.Join(err, closeErr))
	}

	// Close the writer before rename so platforms that disallow
	// renaming an open file (Windows) can complete the publish.
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tempPath, err)
	}

	if err := v.FS.Chmod(tempPath, srcInfo.Mode().Perm()&^WriteBitMask); err != nil {
		return fmt.Errorf("chmod %s: %w", tempPath, err)
	}

	if err := v.FS.Rename(tempPath, path); err != nil {
		return fmt.Errorf("finalize %s: %w", path, err)
	}

	renamed = true

	return nil
}

// GetTmpHandle returns a file handle to a temporary file where content will be stored.
func (c *Content) GetTmpHandle(v Venv, hash string) (vfs.File, error) {
	partitionDir := c.getPartition(hash)
	if err := v.FS.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("create partition dir %s: %w", partitionDir, ErrCreateDir)
	}

	path := c.getPath(hash)
	tempPath := path + ".tmp"

	f, err := v.FS.Create(tempPath)
	if err != nil {
		return nil, fmt.Errorf("create temp file %s: %w", tempPath, err)
	}

	return f, err
}

// Read retrieves content from the store by hash.
func (c *Content) Read(v Venv, hash string) ([]byte, error) {
	path := c.getPath(hash)
	return vfs.ReadFile(v.FS, path)
}

// writeContentToFile writes data to a temporary file, sets appropriate
// permissions, and performs an atomic rename.
func (c *Content) writeContentToFile(l log.Logger, v Venv, hash string, data []byte) error {
	path := c.getPath(hash)
	tempPath := path + ".tmp"

	f, err := v.FS.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, RegularFilePerms)
	if err != nil {
		return fmt.Errorf("create temp file %s: %w", tempPath, err)
	}

	buf := bufio.NewWriter(f)

	if _, err := buf.Write(data); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			l.Warnf("failed to close temp file %s: %v", tempPath, closeErr)
		}

		if removeErr := v.FS.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("write to %s: %w", tempPath, err)
	}

	if err := buf.Flush(); err != nil {
		if closeErr := f.Close(); closeErr != nil {
			l.Warnf("failed to close temp file %s: %v", tempPath, closeErr)
		}

		if removeErr := v.FS.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("flush %s: %w", tempPath, err)
	}

	if err := f.Close(); err != nil {
		if removeErr := v.FS.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("close %s: %w", tempPath, err)
	}

	if err := v.FS.Chmod(tempPath, StoredFilePerms); err != nil {
		if removeErr := v.FS.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("chmod temp %s: %w", tempPath, err)
	}

	if runtime.GOOS == WindowsOS {
		if _, err := v.FS.Stat(path); err == nil {
			if err := v.FS.Chmod(path, RegularFilePerms); err != nil {
				l.Warnf("failed to make destination file writable %s: %v", path, err)
			}
		}
	}

	if err := v.FS.Rename(tempPath, path); err != nil {
		if removeErr := v.FS.Remove(tempPath); removeErr != nil {
			l.Warnf("failed to remove temp file %s: %v", tempPath, removeErr)
		}

		return fmt.Errorf("finalize %s: %w", path, err)
	}

	if runtime.GOOS == WindowsOS {
		if err := v.FS.Chmod(path, StoredFilePerms); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}
	}

	return nil
}

// getPartition returns the partition path for a given hash.
func (c *Content) getPartition(hash string) string {
	return filepath.Join(c.store.Path(), hash[:2])
}

// getPath returns the full path for a given hash.
func (c *Content) getPath(hash string) string {
	return filepath.Join(c.getPartition(hash), hash)
}

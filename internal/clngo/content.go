package clngo

import (
	"os"
	"path/filepath"
)

// Content manages git object storage and linking
type Content struct {
	store *Store
}

const (
	// DefaultDirPerms represents standard directory permissions (rwxr-xr-x)
	DefaultDirPerms = os.FileMode(0755)
	// StoredFilePerms represents read-only file permissions (r--r--r--)
	StoredFilePerms = os.FileMode(0444)
)

// NewContent creates a new Content instance
func NewContent(store *Store) *Content {
	return &Content{store: store}
}

// Link creates a hard link from the store to the target path
func (c *Content) Link(hash, targetPath string) error {
	if err := c.ensureTargetDirectory(targetPath); err != nil {
		return err
	}

	sourcePath := filepath.Join(c.store.Path(), hash)

	// Check if target exists
	if _, err := os.Stat(targetPath); err == nil {
		// File exists, skip creating link
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return &WrappedError{
			Op:   "stat_target",
			Path: targetPath,
			Err:  ErrReadFile,
		}
	}

	// Create hard link since target doesn't exist
	if err := os.Link(sourcePath, targetPath); err != nil {
		return &WrappedError{
			Op:   "create_hard_link",
			Path: targetPath,
			Err:  ErrHardLink,
		}
	}

	return nil
}

func (c *Content) ensureTargetDirectory(targetPath string) error {
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, DefaultDirPerms); err != nil {
		return &WrappedError{
			Op:   "create_target_dir",
			Path: targetDir,
			Err:  ErrCreateDir,
		}
	}
	return nil
}

// Store stores content in the content store
func (c *Content) Store(hash string, data []byte) error {
	path := filepath.Join(c.store.Path(), hash)

	// Check if content already exists
	if c.store.HasContent(hash) {
		return nil
	}

	// Ensure store directory exists
	if err := os.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return &WrappedError{
			Op:   "create_store_dir",
			Path: c.store.Path(),
			Err:  ErrCreateDir,
		}
	}

	// Write content to store with read-only permissions
	if err := os.WriteFile(path, data, StoredFilePerms); err != nil {
		return &WrappedError{
			Op:   "write_to_store",
			Path: path,
			Err:  ErrWriteToStore,
		}
	}

	return nil
}

package clngo

import (
	"bufio"
	"os"
	"path/filepath"
	"sync"
)

// Content manages git object storage and linking
type Content struct {
	store *Store
	mu    sync.RWMutex
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

// Store stores a single content item
func (c *Content) Store(hash string, data []byte) error {
	// Quick check with read lock first
	c.mu.RLock()
	if c.store.HasContent(hash) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	// If content might not exist, take write lock
	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.store.HasContent(hash) {
		return nil
	}

	if err := os.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return wrapError("create_store_dir", c.store.Path(), ErrCreateDir)
	}

	path := filepath.Join(c.store.Path(), hash)
	tempPath := path + ".tmp"

	// Write to temporary file first
	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, StoredFilePerms)
	if err != nil {
		return wrapError("open_temp_file", tempPath, err)
	}

	buf := bufio.NewWriter(f)
	if _, err := buf.Write(data); err != nil {
		f.Close()
		os.Remove(tempPath)
		return wrapError("write_to_store", tempPath, err)
	}

	if err := buf.Flush(); err != nil {
		f.Close()
		os.Remove(tempPath)
		return wrapError("flush_buffer", tempPath, err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tempPath)
		return wrapError("close_file", tempPath, err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return wrapError("finalize_store", path, err)
	}

	return nil
}

// StoreBatch stores multiple content items efficiently using buffered writes
func (c *Content) StoreBatch(items map[string][]byte) error {
	if err := os.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return wrapError("create_store_dir", c.store.Path(), ErrCreateDir)
	}

	// Process items sequentially to avoid race conditions
	for hash, data := range items {
		if c.store.HasContent(hash) {
			continue
		}

		path := filepath.Join(c.store.Path(), hash)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, StoredFilePerms)
		if err != nil {
			return wrapError("open_file", path, err)
		}

		buf := bufio.NewWriter(f)
		if _, err := buf.Write(data); err != nil {
			f.Close()
			return wrapError("write_to_store", path, err)
		}

		if err := buf.Flush(); err != nil {
			f.Close()
			return wrapError("flush_buffer", path, err)
		}

		if err := f.Close(); err != nil {
			return wrapError("close_file", path, err)
		}
	}

	return nil
}

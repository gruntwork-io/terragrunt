package cas

import (
	"bufio"
	"os"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"
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
	// RegularFilePerms represents standard file permissions (rw-r--r--)
	RegularFilePerms = os.FileMode(0644)
)

// NewContent creates a new Content instance
func NewContent(store *Store) *Content {
	return &Content{
		store: store,
	}
}

// Link creates a hard link from the store to the target path
func (c *Content) Link(hash, targetPath string) error {
	if err := c.ensureTargetDirectory(targetPath); err != nil {
		return err
	}

	sourcePath := c.getPath(hash)

	// Check if target exists
	if _, err := os.Stat(targetPath); err == nil {
		// File exists, skip creating link
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return &WrappedError{
			Op:   "stat_target",
			Path: targetPath,
			Err:  err,
		}
	}

	// Create hard link
	if err := os.Link(sourcePath, targetPath); err != nil {
		// If hard link fails, try to copy the file
		data, readErr := os.ReadFile(sourcePath)

		if readErr != nil {
			return &WrappedError{
				Op:   "read_source",
				Path: sourcePath,
				Err:  ErrReadFile,
			}
		}

		// Write to temporary file first
		tempPath := targetPath + ".tmp"

		if err := os.WriteFile(tempPath, data, RegularFilePerms); err != nil {
			return &WrappedError{
				Op:   "write_target",
				Path: tempPath,
				Err:  err,
			}
		}

		// Rename to final path
		if err := os.Rename(tempPath, targetPath); err != nil {
			return &WrappedError{
				Op:   "rename_target",
				Path: tempPath,
				Err:  err,
			}
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
func (c *Content) Store(l *log.Logger, hash string, data []byte) error {
	c.store.mapLock.Lock()

	if _, ok := c.store.locks[hash]; !ok {
		c.store.locks[hash] = &sync.Mutex{}
	}

	c.store.locks[hash].Lock()
	defer c.store.locks[hash].Unlock()

	c.store.mapLock.Unlock()

	if c.store.HasContent(hash) {
		return nil
	}

	if err := os.MkdirAll(c.store.Path(), DefaultDirPerms); err != nil {
		return wrapError("create_store_dir", c.store.Path(), ErrCreateDir)
	}

	// Ensure partition directory exists
	partitionDir := c.getPartition(hash)
	if err := os.MkdirAll(partitionDir, DefaultDirPerms); err != nil {
		return wrapError("create_partition_dir", partitionDir, ErrCreateDir)
	}

	path := c.getPath(hash)
	tempPath := path + ".tmp"

	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, StoredFilePerms)
	if err != nil {
		return wrapError("create_temp_file", tempPath, err)
	}

	buf := bufio.NewWriter(f)

	if _, err := buf.Write(data); err != nil {
		f.Close()

		if err := os.Remove(tempPath); err != nil {
			(*l).Warnf("failed to remove temp file %s: %v", tempPath, err)
		}

		return wrapError("write_to_store", tempPath, err)
	}

	if err := buf.Flush(); err != nil {
		f.Close()

		if err := os.Remove(tempPath); err != nil {
			(*l).Warnf("failed to remove temp file %s: %v", tempPath, err)
		}

		return wrapError("flush_buffer", tempPath, err)
	}

	if err := f.Close(); err != nil {
		if err := os.Remove(tempPath); err != nil {
			(*l).Warnf("failed to remove temp file %s: %v", tempPath, err)
		}

		return wrapError("close_file", tempPath, err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, path); err != nil {
		if err := os.Remove(tempPath); err != nil {
			(*l).Warnf("failed to remove temp file %s: %v", tempPath, err)
		}

		return wrapError("finalize_store", path, err)
	}

	return nil
}

// StoreBatch stores multiple content items efficiently using parallel writes
func (c *Content) StoreBatch(l *log.Logger, items map[string][]byte) error {
	var wg sync.WaitGroup

	errChan := make(chan error, len(items))

	for hash, data := range items {
		wg.Add(1)

		go func(h string, d []byte) {
			defer wg.Done()

			if err := c.Store(l, h, d); err != nil {
				errChan <- err
			}
		}(hash, data)
	}

	wg.Wait()
	close(errChan)

	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

// Read retrieves content from the store by hash
func (c *Content) Read(hash string) ([]byte, error) {
	path := c.getPath(hash)
	return os.ReadFile(path)
}

// getPartition returns the partition path for a given hash
func (c *Content) getPartition(hash string) string {
	return filepath.Join(c.store.Path(), hash[:2])
}

// getPath returns the full path for a given hash
func (c *Content) getPath(hash string) string {
	return filepath.Join(c.getPartition(hash), hash)
}

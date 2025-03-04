// Package cas implements a content-addressable storage for git content.
//
// Blobs are copied from cloned repositories to a local store, along with trees.
// When the same content is requested again, the content is read from the local store,
// avoiding the need to clone the repository or read from the network.
package cas

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	dirPermissions = 0755
)

// Options configures the behavior of the CAS operation
type Options struct {
	// Dir specifies the target directory for the clone
	// If empty, uses the repository name
	Dir string

	// Branch specifies which branch to clone
	// If empty, uses HEAD
	Branch string

	// StorePath specifies a custom path for the content store
	// If empty, uses $HOME/.cache/terragrunt/cas-store
	StorePath string

	// IncludedGitFiles specifies the files to preserve from the .git directory
	// If empty, does not preserve any files
	IncludedGitFiles []string
}

// CAS clones a git repository using content-addressable storage.
// If the content already exists in the store, it creates hard links instead of copying files.
type CAS struct {
	store     *Store
	git       *GitRunner
	opts      Options
	url       string
	cloneLock sync.Mutex
}

// New creates a new CAS instance with the given options
func New(url string, opts Options) (*CAS, error) {
	if opts.StorePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		opts.StorePath = filepath.Join(home, ".cache", "terragrunt", "cas", "store")
	}

	store := NewStore(opts.StorePath)

	return &CAS{
		store: store,
		git:   NewGitRunner(),
		opts:  opts,
		url:   url,
	}, nil
}

// Clone performs the clone operation
func (c *CAS) Clone(ctx context.Context, l *log.Logger) error {
	c.cloneLock.Lock()
	defer c.cloneLock.Unlock()

	targetDir := c.prepareTargetDirectory()

	// Create a temporary directory for git operations
	tempDir, cleanup, err := c.git.CreateTempDir()
	if err != nil {
		return err
	}

	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			(*l).Warnf("cleanup error: %v", cleanupErr)
		}
	}()

	// Set the working directory for git operations
	c.git.SetWorkDir(tempDir)

	hash, err := c.resolveReference()
	if err != nil {
		return err
	}

	if !c.store.HasContent(hash) {
		if err := c.cloneAndStoreContent(l, hash); err != nil {
			return err
		}
	}

	content := NewContent(c.store)

	treeData, err := content.Read(hash)
	if err != nil {
		return err
	}

	tree, err := ParseTree(string(treeData), targetDir)
	if err != nil {
		return err
	}

	if err := tree.LinkTree(ctx, c.store, targetDir); err != nil {
		return err
	}

	return nil
}

func (c *CAS) prepareTargetDirectory() string {
	targetDir := c.opts.Dir
	if targetDir == "" {
		targetDir = GetRepoName(c.url)
	}

	return filepath.Clean(targetDir)
}

func (c *CAS) resolveReference() (string, error) {
	results, err := c.git.LsRemote(c.url, c.opts.Branch)
	if err != nil {
		return "", err
	}

	if len(results) == 0 {
		return "", &WrappedError{
			Op:      "clone",
			Context: "no matching reference",
			Err:     ErrNoMatchingReference,
		}
	}

	return results[0].Hash, nil
}

func (c *CAS) cloneAndStoreContent(l *log.Logger, hash string) error {
	if err := c.git.Clone(c.url, true, 1, c.opts.Branch); err != nil {
		return err
	}

	return c.storeRootTree(l, hash)
}

func (c *CAS) storeRootTree(l *log.Logger, hash string) error {
	if err := c.storeTree(l, hash, ""); err != nil {
		return err
	}

	if len(c.opts.IncludedGitFiles) == 0 {
		return nil
	}

	content := NewContent(c.store)
	data, err := content.Read(hash)
	if err != nil {
		return err
	}

	for _, file := range c.opts.IncludedGitFiles {
		stat, err := os.Stat(filepath.Join(c.git.WorkDir, file))
		if err != nil {
			return err
		}

		if stat.IsDir() {
			continue
		}

		workDirPath := filepath.Join(c.git.WorkDir, file)

		fData, err := os.ReadFile(workDirPath)
		if err != nil {
			return err
		}

		hash, err := hashFile(workDirPath)
		if err != nil {
			return err
		}

		content := NewContent(c.store)

		if err := content.Ensure(l, hash, fData); err != nil {
			return err
		}

		path := filepath.Join(".git", file)

		data = append(data, []byte(fmt.Sprintf("%06o blob %s\t%s\n", stat.Mode().Perm(), hash, path))...)
	}

	// Overwrite the root tree with the new data
	return content.Store(l, hash, data)
}

func (c *CAS) storeTree(l *log.Logger, hash, prefix string) error {
	if c.store.HasContent(hash) {
		return nil
	}

	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	// Optimistically assume half the entries are trees and half are blobs
	var (
		trees = make([]TreeEntry, 0, len(tree.Entries())/2) //nolint:mnd
		blobs = make([]TreeEntry, 0, len(tree.Entries())/2) //nolint:mnd
	)

	for _, entry := range tree.Entries() {
		if prefix != "" {
			entry.Path = filepath.Join(prefix, entry.Path)
		}

		switch entry.Type {
		case "blob":
			blobs = append(blobs, entry)
		case "tree":
			trees = append(trees, entry)
		}
	}

	ch := make(chan error, 2) //nolint:mnd

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := c.storeBlobs(blobs); err != nil {
			ch <- err

			return
		}

		ch <- nil
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		if err := c.storeTrees(l, trees, prefix); err != nil {
			ch <- err

			return
		}

		ch <- nil
	}()

	wg.Wait()

	close(ch)

	errs := []error{}

	for err := range ch {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	content := NewContent(c.store)

	if err := content.Ensure(l, hash, tree.Data()); err != nil {
		return err
	}

	return nil
}

// storeBlobs concurrently stores blobs in the CAS
func (c *CAS) storeBlobs(entries []TreeEntry) error {
	ch := make(chan error, len(entries))

	var wg sync.WaitGroup

	for _, entry := range entries {
		if c.store.HasContent(entry.Hash) {
			continue
		}

		wg.Add(1)

		go func(hash string) {
			defer wg.Done()

			if err := c.ensureBlob(hash); err != nil {
				ch <- err

				return
			}

			ch <- nil
		}(entry.Hash)
	}

	wg.Wait()

	close(ch)

	errs := []error{}

	for err := range ch {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// storeTrees concurrently stores trees in the CAS
func (c *CAS) storeTrees(l *log.Logger, entries []TreeEntry, prefix string) error {
	ch := make(chan error, len(entries))

	var wg sync.WaitGroup

	for _, entry := range entries {
		if c.store.HasContent(entry.Hash) {
			continue
		}

		wg.Add(1)

		go func(hash string) {
			defer wg.Done()

			if err := c.storeTree(l, hash, prefix); err != nil {
				ch <- err

				return
			}

			ch <- nil
		}(entry.Hash)
	}

	wg.Wait()

	close(ch)

	errs := []error{}

	for err := range ch {
		if err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

// ensureBlob ensures that a blob exists in the CAS.
// It doesn't use the standard content.Store method because
// we want to take advantage of the ability to write to the
// entry using `git cat-file`.
func (c *CAS) ensureBlob(hash string) (err error) {
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

	content := NewContent(c.store)
	tmpHandle, err := content.GetTmpHandle(hash)
	if err != nil {
		return err
	}

	tmpPath := tmpHandle.Name()

	// We want to make sure we remove the temporary file
	// if we encounter an error
	defer func() {
		if _, osStatErr := os.Stat(tmpPath); osStatErr == nil {
			err = errors.Join(err, os.Remove(tmpPath))
		}
	}()

	err = c.git.CatFile(hash, tmpHandle)
	if err != nil {
		return err
	}

	if err = tmpHandle.Close(); err != nil {
		return err
	}

	if err = os.Rename(tmpPath, content.getPath(hash)); err != nil {
		return err
	}

	if err = os.Chmod(content.getPath(hash), StoredFilePerms); err != nil {
		return err
	}

	return nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}

	defer file.Close()

	h := sha1.New()

	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

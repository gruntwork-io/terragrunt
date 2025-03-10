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
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Options configures the behavior of CAS
type Options struct {
	// StorePath specifies a custom path for the content store
	// If empty, uses $HOME/.cache/terragrunt/cas/store
	StorePath string
}

// CloneOptions configures the behavior of a specific clone operation
type CloneOptions struct {
	// Dir specifies the target directory for the clone
	// If empty, uses the repository name
	Dir string

	// Branch specifies which branch to clone
	// If empty, uses HEAD
	Branch string

	// IncludedGitFiles specifies the files to preserve from the .git directory
	// If empty, does not preserve any files
	IncludedGitFiles []string
}

// CAS clones a git repository using content-addressable storage.
type CAS struct {
	store      *Store
	git        *GitRunner
	opts       Options
	cloneStart time.Time
}

// New creates a new CAS instance with the given options
//
// TODO: Make these options optional
func New(opts Options) (*CAS, error) {
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
	}, nil
}

// Clone performs the clone operation
//
// TODO: Make options optional
func (c *CAS) Clone(ctx context.Context, l *log.Logger, opts CloneOptions, url string) error {
	c.cloneStart = time.Now()

	targetDir := c.prepareTargetDirectory(opts.Dir, url)

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

	hash, err := c.resolveReference(url, opts.Branch)
	if err != nil {
		return err
	}

	if c.store.NeedsWrite(hash, c.cloneStart) {
		if err := c.cloneAndStoreContent(l, opts, url, hash); err != nil {
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

func (c *CAS) prepareTargetDirectory(dir, url string) string {
	targetDir := dir
	if targetDir == "" {
		targetDir = GetRepoName(url)
	}

	return filepath.Clean(targetDir)
}

func (c *CAS) resolveReference(url, branch string) (string, error) {
	results, err := c.git.LsRemote(url, branch)
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

func (c *CAS) cloneAndStoreContent(l *log.Logger, opts CloneOptions, url string, hash string) error {
	if err := c.git.Clone(url, true, 1, opts.Branch); err != nil {
		return err
	}

	return c.storeRootTree(l, hash, opts)
}

func (c *CAS) storeRootTree(l *log.Logger, hash string, opts CloneOptions) error {
	if err := c.storeTree(l, hash, ""); err != nil {
		return err
	}

	if len(opts.IncludedGitFiles) == 0 {
		return nil
	}

	content := NewContent(c.store)

	data, err := content.Read(hash)
	if err != nil {
		return err
	}

	for _, file := range opts.IncludedGitFiles {
		stat, err := os.Stat(filepath.Join(c.git.WorkDir, file))
		if err != nil {
			return err
		}

		if stat.IsDir() {
			continue
		}

		workDirPath := filepath.Join(c.git.WorkDir, file)

		hash, err := hashFile(workDirPath)
		if err != nil {
			return err
		}

		content := NewContent(c.store)

		if err := content.EnsureCopy(l, hash, workDirPath); err != nil {
			return err
		}

		path := filepath.Join(".git", file)

		data = append(data, []byte(fmt.Sprintf("%06o blob %s\t%s\n", stat.Mode().Perm(), hash, path))...)
	}

	// Overwrite the root tree with the new data
	return content.Store(l, hash, data)
}

func (c *CAS) storeTree(l *log.Logger, hash, prefix string) error {
	if !c.store.NeedsWrite(hash, c.cloneStart) {
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
		if !c.store.NeedsWrite(entry.Hash, c.cloneStart) {
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
		if !c.store.NeedsWrite(entry.Hash, c.cloneStart) {
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

	if !c.store.NeedsWrite(hash, c.cloneStart) {
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

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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
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
	store *Store
	git   *GitRunner
	opts  Options
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

	git, err := NewGitRunner()
	if err != nil {
		return nil, err
	}

	return &CAS{
		store: store,
		git:   git,
		opts:  opts,
	}, nil
}

// Clone performs the clone operation
//
// TODO: Make options optional
func (c *CAS) Clone(ctx context.Context, l log.Logger, opts *CloneOptions, url string) error {
	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "cas_clone", map[string]any{
		"url":    url,
		"branch": opts.Branch,
	}, func(childCtx context.Context) error {
		targetDir := c.prepareTargetDirectory(opts.Dir, url)

		// Create a temporary directory for git operations
		tempDir, cleanup, err := c.git.CreateTempDir()
		if err != nil {
			return err
		}

		defer func() {
			if cleanupErr := cleanup(); cleanupErr != nil {
				l.Warnf("cleanup error: %v", cleanupErr)
			}
		}()

		// Set the working directory for git operations
		c.git.SetWorkDir(tempDir)

		hash, err := c.resolveReference(childCtx, url, opts.Branch)
		if err != nil {
			return err
		}

		if c.store.NeedsWrite(hash) {
			if err := c.cloneAndStoreContent(childCtx, l, opts, url, hash); err != nil {
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

		return tree.LinkTree(childCtx, c.store, targetDir)
	})
}

func (c *CAS) prepareTargetDirectory(dir, url string) string {
	targetDir := dir
	if targetDir == "" {
		targetDir = GetRepoName(url)
	}

	return filepath.Clean(targetDir)
}

func (c *CAS) resolveReference(ctx context.Context, url, branch string) (string, error) {
	results, err := c.git.LsRemote(ctx, url, branch)
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

func (c *CAS) cloneAndStoreContent(ctx context.Context, l log.Logger, opts *CloneOptions, url string, hash string) error {
	if err := c.git.Clone(ctx, url, true, 1, opts.Branch); err != nil {
		return err
	}

	return c.storeRootTree(ctx, l, hash, opts)
}

func (c *CAS) storeRootTree(ctx context.Context, l log.Logger, hash string, opts *CloneOptions) error {
	// Get all blobs recursively in a single git ls-tree -r call at the root
	allBlobsTree, err := c.git.LsTreeRecursive(ctx, hash, ".")
	if err != nil {
		return err
	}

	// Collect all blobs from recursive listing
	var allBlobs []TreeEntry

	for _, entry := range allBlobsTree.Entries() {
		if entry.Type == "blob" {
			allBlobs = append(allBlobs, entry)
		}
	}

	// Store all blobs concurrently (single batch from recursive call)
	if err := c.storeBlobs(ctx, allBlobs); err != nil {
		return err
	}

	// Now store the tree structure (which won't need to handle blobs again)
	if err := c.storeTree(ctx, l, hash, ""); err != nil {
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

		data = append(data, fmt.Appendf(nil, "%06o blob %s\t%s\n", stat.Mode().Perm(), hash, path)...)
	}

	// Overwrite the root tree with the new data
	return content.Store(l, hash, data)
}

func (c *CAS) storeTree(ctx context.Context, l log.Logger, hash, prefix string) error {
	if !c.store.NeedsWrite(hash) {
		return nil
	}

	// Get tree structure (no recursive blobs needed - they're already stored)
	tree, err := c.git.LsTree(ctx, hash, ".")
	if err != nil {
		return err
	}

	// Only collect immediate tree entries (blobs are already handled at root)
	var immediateTrees []TreeEntry

	for _, entry := range tree.Entries() {
		if prefix != "" {
			entry.Path = filepath.Join(prefix, entry.Path)
		}

		if entry.Type == "tree" {
			immediateTrees = append(immediateTrees, entry)
		}
	}

	// Store tree objects recursively
	if err := c.storeTrees(ctx, l, immediateTrees, prefix); err != nil {
		return err
	}

	// Store the current tree object
	content := NewContent(c.store)
	if err := content.EnsureWithWait(l, hash, tree.Data()); err != nil {
		return err
	}

	return nil
}

// storeTrees stores trees in the CAS
func (c *CAS) storeTrees(ctx context.Context, l log.Logger, entries []TreeEntry, prefix string) error {
	for _, entry := range entries {
		if !c.store.NeedsWrite(entry.Hash) {
			continue
		}

		if err := c.storeTree(ctx, l, entry.Hash, prefix); err != nil {
			return err
		}
	}

	return nil
}

// storeBlobs stores blobs in the CAS
func (c *CAS) storeBlobs(ctx context.Context, entries []TreeEntry) error {
	for _, entry := range entries {
		if !c.store.NeedsWrite(entry.Hash) {
			continue
		}

		if err := c.ensureBlob(ctx, entry.Hash); err != nil {
			return err
		}
	}

	return nil
}

// ensureBlob ensures that a blob exists in the CAS.
// It doesn't use the standard content.Store method because
// we want to take advantage of the ability to write to the
// entry using `git cat-file`.
func (c *CAS) ensureBlob(ctx context.Context, hash string) error {
	needsWrite, lock, err := c.store.EnsureWithWait(hash)
	if err != nil {
		return err
	}

	// If content already exists or was written by another process, we're done
	if !needsWrite {
		return nil
	}

	// We have the lock and need to write the content
	defer func() {
		if unlockErr := lock.Unlock(); unlockErr != nil {
			err = errors.Join(err, unlockErr)
		}
	}()

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

	err = c.git.CatFile(ctx, hash, tmpHandle)
	if err != nil {
		return err
	}

	// For Windows, ensure data is synchronized to disk
	if runtime.GOOS == "windows" {
		if err = tmpHandle.Sync(); err != nil {
			return err
		}
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

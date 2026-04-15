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
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const defaultCloneDepth = 1

// Option configures the behavior of CAS.
type Option func(*CAS)

// CloneOptions configures the behavior of a specific clone operation.
type CloneOptions struct {
	// Dir specifies the target directory for the clone.
	// If empty, uses the repository name.
	Dir string

	// Branch specifies which branch to clone.
	// If empty, uses HEAD.
	Branch string

	// IncludedGitFiles specifies the files to preserve from the .git directory.
	// If empty, does not preserve any files.
	IncludedGitFiles []string

	// Depth limits the clone history to the given number of commits.
	// If zero, defaults to 1 (shallow clone). Set to -1 for full history.
	Depth int
}

// CAS clones a git repository using content-addressable storage.
type CAS struct {
	fs         vfs.FS
	blobStore  *Store
	treeStore  *Store
	synthStore *Store
	git        *git.GitRunner
	storePath  string
	cloneDepth int
}

// WithStorePath specifies a custom path for the content store.
// If not set, defaults to $HOME/.cache/terragrunt/cas/store.
func WithStorePath(path string) Option {
	return func(c *CAS) {
		c.storePath = path
	}
}

// WithCloneDepth overrides the default clone depth.
// A value of 0 uses the default (shallow clone, depth 1).
// A negative value (e.g. -1) clones the full history.
func WithCloneDepth(depth int) Option {
	return func(c *CAS) {
		c.cloneDepth = depth
	}
}

// WithFS specifies the filesystem for file operations.
// If not set, defaults to the real OS filesystem.
func WithFS(fs vfs.FS) Option {
	return func(c *CAS) {
		c.fs = fs
	}
}

// New creates a new CAS instance with the given options.
func New(opts ...Option) (*CAS, error) {
	c := &CAS{}

	for _, opt := range opts {
		opt(c)
	}

	if c.fs == nil {
		c.fs = vfs.NewOSFS()
	}

	if c.storePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		c.storePath = filepath.Join(home, ".cache", "terragrunt", "cas", "store")
	}

	if err := c.fs.MkdirAll(c.storePath, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("failed to create CAS store path: %w", err)
	}

	c.blobStore = NewStore(filepath.Join(c.storePath, "blobs")).WithFS(c.fs)
	c.treeStore = NewStore(filepath.Join(c.storePath, "trees")).WithFS(c.fs)
	c.synthStore = NewStore(filepath.Join(c.storePath, "synth", "trees")).WithFS(c.fs)

	for _, s := range []*Store{c.blobStore, c.treeStore, c.synthStore} {
		if err := c.fs.MkdirAll(s.Path(), DefaultDirPerms); err != nil {
			return nil, fmt.Errorf("failed to create CAS store subdirectory %s: %w", s.Path(), err)
		}
	}

	g, err := git.NewGitRunner(vexec.NewOSExec())
	if err != nil {
		return nil, err
	}

	c.git = g

	return c, nil
}

// FS returns the configured filesystem.
func (c *CAS) FS() vfs.FS {
	return c.fs
}

// BlobStore returns the store for blob content.
func (c *CAS) BlobStore() *Store { return c.blobStore }

// TreeStore returns the store for git-derived tree content.
func (c *CAS) TreeStore() *Store { return c.treeStore }

// SynthStore returns the store for synthetic tree content.
func (c *CAS) SynthStore() *Store { return c.synthStore }

// Clone performs the clone operation
//
// TODO: Make options optional
func (c *CAS) Clone(ctx context.Context, l log.Logger, opts *CloneOptions, url string) error {
	// Ensure the store paths exist
	if err := c.fs.MkdirAll(c.blobStore.Path(), DefaultDirPerms); err != nil {
		return fmt.Errorf("failed to create blob store path: %w", err)
	}

	if err := c.fs.MkdirAll(c.treeStore.Path(), DefaultDirPerms); err != nil {
		return fmt.Errorf("failed to create tree store path: %w", err)
	}

	// Acquire global clone lock to ensure only one clone at a time
	globalLock, err := vfs.Lock(c.fs, filepath.Join(c.storePath, "clone.lock"))
	if err != nil {
		return fmt.Errorf("failed to acquire global clone lock: %w", err)
	}

	defer func() {
		if unlockErr := globalLock.Unlock(); unlockErr != nil {
			l.Warnf("failed to release global clone lock: %v", unlockErr)
		}
	}()

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, "cas_clone", map[string]any{
		"url":    url,
		"branch": opts.Branch,
	}, func(childCtx context.Context) error {
		hash, err := c.resolveReference(childCtx, url, opts.Branch)
		if err != nil {
			return err
		}

		targetDir := c.prepareTargetDirectory(opts.Dir, url)

		if c.treeStore.NeedsWrite(hash) {
			// Create a temporary directory for git operations
			_, cleanup, createTempDirErr := c.git.CreateTempDir()
			if createTempDirErr != nil {
				return createTempDirErr
			}

			defer func() {
				if cleanupErr := cleanup(); cleanupErr != nil {
					l.Warnf("cleanup error: %v", cleanupErr)
				}
			}()

			if cloneAndStoreErr := c.cloneAndStoreContent(childCtx, l, opts, url, hash); cloneAndStoreErr != nil {
				return cloneAndStoreErr
			}
		}

		treeContent := NewContent(c.treeStore)

		treeData, err := treeContent.Read(hash)
		if err != nil {
			return err
		}

		tree, err := git.ParseTree(treeData, targetDir)
		if err != nil {
			return err
		}

		return LinkTree(childCtx, c.blobStore, c.treeStore, tree, targetDir)
	})
}

func (c *CAS) prepareTargetDirectory(dir, url string) string {
	targetDir := dir
	if targetDir == "" {
		targetDir = git.ExtractRepoName(url)
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

func (c *CAS) cloneAndStoreContent(
	ctx context.Context,
	l log.Logger,
	opts *CloneOptions,
	url,
	hash string,
) error {
	depth := opts.Depth
	if depth == 0 {
		depth = defaultCloneDepth
	}

	if depth < 0 {
		depth = 0
	}

	if err := c.git.Clone(ctx, url, true, depth, opts.Branch); err != nil {
		return err
	}

	return c.storeRootTree(ctx, l, hash, opts)
}

func (c *CAS) storeRootTree(ctx context.Context, l log.Logger, hash string, opts *CloneOptions) error {
	tree, err := c.git.LsTreeRecursive(ctx, hash)
	if err != nil {
		return err
	}

	if err = c.storeTreeRecursive(ctx, l, hash, tree); err != nil {
		return err
	}

	if len(opts.IncludedGitFiles) == 0 {
		return nil
	}

	treeContent := NewContent(c.treeStore)

	data, err := treeContent.Read(hash)
	if err != nil {
		return err
	}

	for _, file := range opts.IncludedGitFiles {
		stat, err := c.fs.Stat(filepath.Join(c.git.WorkDir, file))
		if err != nil {
			return err
		}

		if stat.IsDir() {
			continue
		}

		workDirPath := filepath.Join(c.git.WorkDir, file)

		includedHash, err := hashFile(c.fs, workDirPath)
		if err != nil {
			return err
		}

		blobContent := NewContent(c.blobStore)

		if err := blobContent.EnsureCopy(l, includedHash, workDirPath); err != nil {
			return err
		}

		path := filepath.Join(".git", file)

		data = append(data, fmt.Appendf(nil, "%06o blob %s\t%s\n", stat.Mode().Perm(), includedHash, path)...)
	}

	// Overwrite the root tree with the new data
	return treeContent.Store(l, hash, data)
}

// storeTreeRecursive stores a tree fetched from git ls-tree -r
func (c *CAS) storeTreeRecursive(ctx context.Context, l log.Logger, hash string, tree *git.Tree) error {
	if !c.treeStore.NeedsWrite(hash) {
		return nil
	}

	if err := c.storeBlobs(ctx, tree.Entries()); err != nil {
		return err
	}

	// Store the tree object itself
	treeContent := NewContent(c.treeStore)
	if err := treeContent.EnsureWithWait(l, hash, tree.Data()); err != nil {
		return err
	}

	return nil
}

// storeBlobs stores blobs in the CAS
func (c *CAS) storeBlobs(ctx context.Context, entries []git.TreeEntry) error {
	for _, entry := range entries {
		if !c.blobStore.NeedsWrite(entry.Hash) {
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
	needsWrite, lock, err := c.blobStore.EnsureWithWait(hash)
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

	content := NewContent(c.blobStore)

	tmpHandle, err := content.GetTmpHandle(hash)
	if err != nil {
		return err
	}

	tmpPath := tmpHandle.Name()

	// We want to make sure we remove the temporary file
	// if we encounter an error
	defer func() {
		if _, statErr := c.fs.Stat(tmpPath); statErr == nil {
			err = errors.Join(err, c.fs.Remove(tmpPath))
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

	if err = c.fs.Rename(tmpPath, content.getPath(hash)); err != nil {
		return err
	}

	if err = c.fs.Chmod(content.getPath(hash), StoredFilePerms); err != nil {
		return err
	}

	return nil
}

func hashFile(fs vfs.FS, path string) (string, error) {
	file, err := fs.Open(path)
	if err != nil {
		return "", err
	}

	h := sha1.New()

	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	hash := hex.EncodeToString(h.Sum(nil))

	if err := file.Close(); err != nil {
		return hash, fmt.Errorf(
			"hash of %s successfully computed as "+
				"%s, but closing the file failed: %w",
			path,
			hash,
			err,
		)
	}

	return hash, nil
}

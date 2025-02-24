// Package clngo is a golang port of the cln project.
//
// The original project is written in Rust and can be found at:
// https://github.com/yhakbar/cln
package clngo

import (
	"os"
	"path/filepath"
	"sync"
)

// Options configures the behavior of the Cln operation
type Options struct {
	// Dir specifies the target directory for the clone
	// If empty, uses the repository name
	Dir string

	// Branch specifies which branch to clone
	// If empty, uses HEAD
	Branch string

	// StorePath specifies a custom path for the content store
	// If empty, uses $HOME/.cache/.cln-store
	StorePath string
}

// Cln clones a git repository using content-addressable storage.
// If the content already exists in the store, it creates hard links instead of copying files.
type Cln struct {
	store     *Store
	git       *GitRunner
	opts      Options
	repo      string
	cloneLock sync.Mutex
}

// New creates a new Cln instance with the given options
func New(repo string, opts Options) (*Cln, error) {
	store, err := NewStore(opts.StorePath)
	if err != nil {
		return nil, err
	}

	return &Cln{
		store: store,
		git:   NewGitRunner(),
		opts:  opts,
		repo:  repo,
	}, nil
}

// Clone performs the clone operation
func (c *Cln) Clone() error {
	c.cloneLock.Lock()
	defer c.cloneLock.Unlock()

	targetDir, err := c.prepareTargetDirectory()
	if err != nil {
		return err
	}

	hash, err := c.resolveReference()
	if err != nil {
		return err
	}

	// Check if we have the complete reference
	if c.store.HasReference(hash) {
		return c.linkFromStore(targetDir, hash)
	}

	// If we don't have the reference, do a full clone
	return c.cloneAndStoreContent(targetDir, hash)
}

func (c *Cln) prepareTargetDirectory() (string, error) {
	targetDir := c.opts.Dir
	if targetDir == "" {
		targetDir = GetRepoName(c.repo)
	}
	return filepath.Clean(targetDir), nil
}

func (c *Cln) resolveReference() (string, error) {
	reference := "HEAD"
	if c.opts.Branch != "" {
		reference = c.opts.Branch
	}

	results, err := c.git.LsRemote(c.repo, reference)
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

func (c *Cln) linkFromStore(targetDir, hash string) error {
	// Set up a temporary git repo to read the tree
	tempDir, cleanup, err := c.git.CreateTempDir()
	if err != nil {
		return err
	}
	defer cleanup()

	// Do a bare clone to get the git objects
	if err := c.git.Clone(c.repo, true, 0, ""); err != nil {
		return err
	}

	c.git.SetWorkDir(tempDir)

	// Get the tree structure using git cat-file
	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	content := NewContent(c.store)
	return c.processTreeEntries(targetDir, tree, content)
}

func (c *Cln) cloneAndStoreContent(targetDir, hash string) error {
	// Create temporary directory for initial clone
	tempDir, cleanup, err := c.git.CreateTempDir()
	if err != nil {
		return err
	}
	defer cleanup()

	// Perform shallow clone to temporary directory
	if err := c.git.Clone(c.repo, true, 1, c.opts.Branch); err != nil {
		return err
	}

	c.git.SetWorkDir(tempDir)
	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	// Collect all blob entries first
	var blobEntries []TreeEntry
	for _, entry := range tree.Entries() {
		if entry.Type == "blob" {
			blobEntries = append(blobEntries, entry)
		}
	}

	// Use a worker pool to fetch blobs in parallel
	blobs := make(map[string][]byte)
	var mu sync.Mutex
	errChan := make(chan error, 1)
	semaphore := make(chan struct{}, 4) // Limit concurrent git operations

	var wg sync.WaitGroup
	for _, entry := range blobEntries {
		if c.store.HasContent(entry.Hash) {
			continue
		}

		wg.Add(1)
		go func(hash string) {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			data, err := c.git.CatFile(hash)
			if err != nil {
				select {
				case errChan <- err:
				default:
				}
				return
			}

			mu.Lock()
			blobs[hash] = data
			mu.Unlock()
		}(entry.Hash)
	}

	// Wait for all fetches to complete
	wg.Wait()

	// Check for errors
	select {
	case err := <-errChan:
		return err
	default:
	}

	// Store all blobs in one batch operation
	content := NewContent(c.store)
	if err := content.StoreBatch(blobs); err != nil {
		return err
	}

	// Mark this reference as stored
	if err := c.store.StoreReference(hash); err != nil {
		return err
	}

	return c.processTreeEntries(targetDir, tree, content)
}

func (c *Cln) processTreeEntries(targetDir string, tree *Tree, content *Content) error {
	if err := os.MkdirAll(targetDir, DefaultDirPerms); err != nil {
		return &WrappedError{
			Op:   "create_target_dir",
			Path: targetDir,
			Err:  ErrCreateDir,
		}
	}

	for _, entry := range tree.Entries() {
		if err := c.processTreeEntry(targetDir, entry, content); err != nil {
			return err
		}
	}
	return nil
}

func (c *Cln) processTreeEntry(targetDir string, entry TreeEntry, content *Content) error {
	entryPath := filepath.Join(targetDir, entry.Path)

	if entry.Type == "tree" {
		return os.MkdirAll(entryPath, DefaultDirPerms)
	}

	if err := os.MkdirAll(filepath.Dir(entryPath), DefaultDirPerms); err != nil {
		return &WrappedError{
			Op:   "create_parent_dir",
			Path: filepath.Dir(entryPath),
			Err:  ErrCreateDir,
		}
	}

	if err := content.Link(entry.Hash, entryPath); err != nil {
		return err
	}

	if entry.Mode == "100755" {
		return os.Chmod(entryPath, 0755)
	}
	return nil
}

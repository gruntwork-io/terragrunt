// Package clngo is a golang port of the cln project.
//
// The original project is written in Rust and can be found at:
// https://github.com/yhakbar/cln
package clngo

import (
	"os"
	"path/filepath"
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
	store *Store
	git   *GitRunner
	opts  Options
	repo  string
}

// New creates a new Cln instance with the given options
func New(repo string, opts Options) (*Cln, error) {
	store, err := NewStore(opts.StorePath)
	if err != nil {
		return nil, err
	}

	return &Cln{
		store: store,
		git:   NewGitRunner(""),
		opts:  opts,
		repo:  repo,
	}, nil
}

// Clone performs the clone operation
func (c *Cln) Clone() error {
	targetDir, err := c.prepareTargetDirectory()
	if err != nil {
		return err
	}

	hash, err := c.resolveReference()
	if err != nil {
		return err
	}

	if c.store.HasContent(hash) {
		return c.linkExistingContent(targetDir, hash)
	}

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

func (c *Cln) linkExistingContent(targetDir, hash string) error {
	content := NewContent(c.store)
	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	return c.processTreeEntries(targetDir, tree, content)
}

func (c *Cln) cloneAndStoreContent(targetDir, hash string) error {
	tempDir, cleanup, err := CreateTempDir()
	if err != nil {
		return err
	}
	defer cleanup()

	if err := c.performInitialClone(tempDir); err != nil {
		return err
	}

	return c.storeAndLinkContent(tempDir, targetDir, hash)
}

func (c *Cln) performInitialClone(tempDir string) error {
	c.git.SetWorkDir(tempDir)
	if err := c.git.Clone(c.repo, false, 0, c.opts.Branch); err != nil {
		return err
	}
	return nil
}

func (c *Cln) storeAndLinkContent(tempDir, targetDir, hash string) error {
	c.git.SetWorkDir(tempDir) // Set working directory for git commands
	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	content := NewContent(c.store)
	for _, entry := range tree.Entries() {
		if entry.Type == "blob" {
			data, err := c.git.CatFile(entry.Hash)
			if err != nil {
				return err
			}
			if err := content.Store(entry.Hash, data); err != nil {
				return err
			}
		}
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

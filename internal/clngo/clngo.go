// Package clngo is a golang port of the cln project.
//
// The original project is written in Rust and can be found at:
// https://github.com/yhakbar/cln
package clngo

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	// If empty, uses $HOME/.cln-store
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

	// Strip the git:: prefix if present
	repo = strings.TrimPrefix(repo, "git::")
	// Also strip any ssh:// prefix as git handles SSH URLs without it
	repo = strings.TrimPrefix(repo, "ssh://")

	// Convert github.com/org/repo to github.com:org/repo format for SSH URLs
	if strings.HasPrefix(repo, "git@") {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) == 2 {
			repo = parts[0] + ":" + parts[1]
		}
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

	// Check if we have the tree structure cached
	if treeData, err := c.git.CatFile(hash); err == nil {
		// We have the tree structure, try to link files directly
		tree, err := ParseTree(string(treeData), targetDir)
		if err == nil {
			// Try to link all files from the store
			content := NewContent(c.store)
			allFilesPresent := true

			for _, entry := range tree.Entries() {
				if entry.Type != "blob" {
					continue
				}
				targetPath := filepath.Join(targetDir, entry.Path)
				if err := content.Link(entry.Hash, targetPath); err != nil {
					allFilesPresent = false
					break
				}
			}

			if allFilesPresent {
				return nil // Successfully reused all content
			}
		}
	}

	// Fall back to full clone if optimization fails
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

func (c *Cln) cloneAndStoreContent(targetDir, hash string) error {
	tempDir, cleanup, err := c.git.CreateTempDir()
	if err != nil {
		return err
	}
	defer cleanup()

	if err := c.git.Clone(c.repo, true, 1, c.opts.Branch); err != nil {
		return err
	}

	c.git.SetWorkDir(tempDir)

	return c.storeTreeRecursively(hash, "", targetDir)
}

func (c *Cln) storeTreeRecursively(hash, prefix, targetDir string) error {
	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	// Store the tree data itself
	content := NewContent(c.store)
	treeData := strings.Builder{}
	for _, entry := range tree.Entries() {
		fmt.Fprintf(&treeData, "%s %s %s %s\n", entry.Mode, entry.Type, entry.Hash, entry.Path)
	}
	if err := content.Store(hash, []byte(treeData.String())); err != nil {
		return err
	}

	var blobEntries []TreeEntry
	var subTrees []TreeEntry

	for _, entry := range tree.Entries() {
		if prefix != "" {
			entry.Path = filepath.Join(prefix, entry.Path)
		}

		if entry.Type == "blob" {
			blobEntries = append(blobEntries, entry)
		} else if entry.Type == "tree" {
			subTrees = append(subTrees, entry)
		}
	}

	if err := c.storeBlobEntries(blobEntries); err != nil {
		return err
	}

	for _, subTree := range subTrees {
		if err := c.storeTreeRecursively(subTree.Hash, subTree.Path, targetDir); err != nil {
			return err
		}
	}

	return c.processTreeEntries(targetDir, tree, content)
}

func (c *Cln) storeBlobEntries(entries []TreeEntry) error {
	blobs := make(map[string][]byte)
	var mu sync.Mutex
	errChan := make(chan error, 1)
	semaphore := make(chan struct{}, 4)

	var wg sync.WaitGroup
	for _, entry := range entries {
		if c.store.HasContent(entry.Hash) {
			continue
		}

		wg.Add(1)
		go func(hash string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

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

	wg.Wait()

	select {
	case err := <-errChan:
		return err
	default:
	}

	if len(blobs) > 0 {
		content := NewContent(c.store)
		return content.StoreBatch(blobs)
	}

	return nil
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

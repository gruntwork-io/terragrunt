// Package cln is a golang port of the cln project.
//
// The original project is written in Rust and can be found at:
// https://github.com/gruntwork-io/terragrunt
package cln

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	repoPartsSplitLimit = 2
	maxConcurrentStores = 4
	dirPermissions      = 0755
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
	// If empty, uses $HOME/.cache/terragrunt/cln-store
	StorePath string

	// IncludedGitFiles specifies the files to preserve from the .git directory
	// If empty, does not preserve any files
	IncludedGitFiles []string
}

// Cln clones a git repository using content-addressable storage.
// If the content already exists in the store, it creates hard links instead of copying files.
type Cln struct {
	store     *Store
	git       *GitRunner
	opts      Options
	url       string
	cloneLock sync.Mutex
}

// New creates a new Cln instance with the given options
func New(url string, opts Options) (*Cln, error) {
	if opts.StorePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		opts.StorePath = filepath.Join(home, ".cache", "terragrunt", "cln-store")
	}

	store, err := NewStore(opts.StorePath)
	if err != nil {
		return nil, err
	}

	url = adjustFromGoGetterURL(url)

	return &Cln{
		store: store,
		git:   NewGitRunner(),
		opts:  opts,
		url:   url,
	}, nil
}

// adjustFromGoGetterURL strips the go-getter prefixes from the URL
// That's passed in.
//
// Long term, we should strip them before they're used in this package.
func adjustFromGoGetterURL(url string) string {
	// Strip the cln:// prefix if present - this is just a marker for using CLN
	url = strings.TrimPrefix(url, "cln://")

	// Then strip the git:: prefix if present
	url = strings.TrimPrefix(url, "git::")

	// Also strip any ssh:// prefix as git handles SSH URLs without it
	url = strings.TrimPrefix(url, "ssh://")

	// Convert github.com/org/repo to github.com:org/repo format for SSH URLs
	if strings.HasPrefix(url, "git@") {
		parts := strings.SplitN(url, "/", repoPartsSplitLimit)
		if len(parts) == repoPartsSplitLimit {
			url = parts[0] + ":" + parts[1]
		}
	}

	return url
}

// Clone performs the clone operation
func (c *Cln) Clone() error {
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
			// TODO: Move to proper logger
			log.Printf("cleanup error: %v", cleanupErr)
		}
	}()

	// Set the working directory for git operations
	c.git.SetWorkDir(tempDir)

	hash, err := c.resolveReference()
	if err != nil {
		return err
	}

	if !c.store.HasContent(hash) {
		if err := c.cloneAndStoreContent(hash); err != nil {
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

	if err := tree.LinkTree(c.store, targetDir); err != nil {
		return err
	}

	return nil
}

func (c *Cln) prepareTargetDirectory() string {
	targetDir := c.opts.Dir
	if targetDir == "" {
		targetDir = GetRepoName(c.url)
	}

	return filepath.Clean(targetDir)
}

func (c *Cln) resolveReference() (string, error) {
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

func (c *Cln) cloneAndStoreContent(hash string) error {
	if err := c.git.Clone(c.url, true, 1, c.opts.Branch); err != nil {
		return err
	}

	return c.storeTreeRecursively(hash, "")
}

func (c *Cln) storeTreeRecursively(hash, prefix string) error {
	tree, err := c.git.LsTree(hash, ".")
	if err != nil {
		return err
	}

	content := NewContent(c.store)
	treeData := strings.Builder{}

	for _, entry := range tree.Entries() {
		fmt.Fprintf(&treeData, "%s %s %s %s\n", entry.Mode, entry.Type, entry.Hash, entry.Path)
	}

	var (
		subTrees    []TreeEntry
		blobEntries []TreeEntry
	)

	for _, entry := range tree.Entries() {
		if prefix != "" {
			entry.Path = filepath.Join(prefix, entry.Path)
		}

		switch entry.Type {
		case "blob":
			blobEntries = append(blobEntries, entry)
		case "tree":
			subTrees = append(subTrees, entry)
		}
	}

	if err := c.storeBlobEntries(blobEntries); err != nil {
		return err
	}

	for _, subTree := range subTrees {
		if err := c.storeTreeRecursively(subTree.Hash, subTree.Path); err != nil {
			return err
		}
	}

	// We only do this if we're at the root of the repository.
	if prefix == "" {
		for _, file := range c.opts.IncludedGitFiles {
			stat, err := os.Stat(filepath.Join(c.git.WorkDir, file))
			if err != nil {
				return err
			}

			if stat.IsDir() {
				continue
			}

			workDirPath := filepath.Join(c.git.WorkDir, file)

			data, err := os.ReadFile(workDirPath)
			if err != nil {
				return err
			}

			hash, err := hashFile(workDirPath)
			if err != nil {
				return err
			}

			content := NewContent(c.store)

			if err := content.Store(hash, data); err != nil {
				return err
			}

			path := filepath.Join(".git", file)

			fmt.Fprintf(&treeData, "%s %s %s %s\n", stat.Mode(), "blob", hash, path)
		}
	}

	if err := content.Store(hash, []byte(treeData.String())); err != nil {
		return err
	}

	return nil
}

func (c *Cln) storeBlobEntries(entries []TreeEntry) error {
	blobs := make(map[string][]byte)
	errChan := make(chan error, 1)
	semaphore := make(chan struct{}, maxConcurrentStores)

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

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

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	hash := sha1.Sum(data)

	return hex.EncodeToString(hash[:]), nil
}

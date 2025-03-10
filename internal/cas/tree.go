package cas

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	minTreePartsLength = 4
	maxConcurrentLinks = 4
)

// TreeEntry represents a single entry in a git tree
type TreeEntry struct {
	Mode string
	Type string
	Hash string
	Path string
}

// Tree represents a git tree object with its entries
type Tree struct {
	entries []TreeEntry
	path    string
	data    []byte
}

// Entries returns the tree entries
func (t *Tree) Entries() []TreeEntry {
	return t.entries
}

// Path returns the tree path
func (t *Tree) Path() string {
	return t.path
}

// Data returns the tree data
func (t *Tree) Data() []byte {
	return t.data
}

// ParseTreeEntry parses a single line from git ls-tree output
func ParseTreeEntry(line string) (TreeEntry, error) {
	// Format: <mode> <type> <hash> <path>
	parts := strings.Fields(line)
	if len(parts) < minTreePartsLength {
		return TreeEntry{}, &WrappedError{
			Op:      "parse_tree_entry",
			Context: "invalid tree entry format",
			Err:     ErrParseTree,
		}
	}

	return TreeEntry{
		Mode: parts[0],
		Type: parts[1],
		Hash: parts[2],
		Path: strings.Join(parts[3:], " "), // Handle paths with spaces
	}, nil
}

// ParseTree parses the complete output of git ls-tree
func ParseTree(output, path string) (*Tree, error) {
	// Pre-allocate capacity based on newline count
	capacity := strings.Count(output, "\n") + 1
	entries := make([]TreeEntry, 0, capacity)

	// Use a scanner for more efficient line reading
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		entry, err := ParseTreeEntry(line)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, wrapErrorWithContext("scan_tree", "failed to read tree output", err)
	}

	return &Tree{
		entries: entries,
		path:    path,
		data:    []byte(output),
	}, nil
}

// LinkTree writes the tree to a target directory
func (t *Tree) LinkTree(ctx context.Context, store *Store, targetDir string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		errMu sync.Mutex
		errs  []error
	)

	var wg sync.WaitGroup

	semaphore := make(chan struct{}, maxConcurrentLinks)

	for _, entry := range t.entries {
		wg.Add(1)

		go func(entry TreeEntry) {
			defer wg.Done()

			select {
			case <-ctx.Done():
				return
			default:
			}

			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			entryPath := filepath.Join(targetDir, entry.Path)
			if err := os.MkdirAll(filepath.Dir(entryPath), DefaultDirPerms); err != nil {
				errMu.Lock()
				errs = append(errs, wrapError("mkdir_all", entryPath, err))
				errMu.Unlock()
				cancel()

				return
			}

			content := NewContent(store)

			switch entry.Type {
			case "blob":
				if err := content.Link(entry.Hash, entryPath); err != nil {
					errMu.Lock()
					errs = append(errs, wrapError("link_blob", entryPath, err))
					errMu.Unlock()
					cancel()

					return
				}
			case "tree":
				treeData, err := content.Read(entry.Hash)
				if err != nil {
					errMu.Lock()
					errs = append(errs, wrapError("read_tree", entry.Hash, err))
					errMu.Unlock()
					cancel()

					return
				}

				subTree, err := ParseTree(string(treeData), entryPath)
				if err != nil {
					errMu.Lock()
					errs = append(errs, wrapError("parse_tree", entry.Hash, err))
					errMu.Unlock()
					cancel()

					return
				}

				if err := subTree.LinkTree(ctx, store, entryPath); err != nil {
					errMu.Lock()
					errs = append(errs, wrapError("link_subtree", entryPath, err))
					errMu.Unlock()
					cancel()

					return
				}
			}
		}(entry)
	}

	wg.Wait()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}

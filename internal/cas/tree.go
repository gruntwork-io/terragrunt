package cas

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sync/errgroup"
)

const (
	minTreePartsLength = 4
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
	content := NewContent(store)

	dirsToCreate := make(map[string]struct{}, len(t.entries))

	type workItem struct {
		itemType string
		entry    TreeEntry
		path     string
		dirPath  string
	}

	workItems := make([]workItem, 0, len(t.entries))

	for _, entry := range t.entries {
		entryPath := filepath.Join(targetDir, entry.Path)
		dirPath := filepath.Dir(entryPath)

		dirsToCreate[dirPath] = struct{}{}

		// If the parent directory is in dirsToCreate,
		// we can remove it, since it will be created
		// when creating the subtree anyways.
		parentDirPath := filepath.Dir(dirPath)
		delete(dirsToCreate, parentDirPath)

		// Create work items based on entry type
		switch entry.Type {
		case "blob":
			workItems = append(workItems, workItem{
				itemType: "link",
				entry:    entry,
				path:     entryPath,
				dirPath:  dirPath,
			})
		case "tree":
			workItems = append(workItems, workItem{
				itemType: "subtree",
				entry:    entry,
				path:     entryPath,
				dirPath:  dirPath,
			})
		}
	}

	for dirPath := range dirsToCreate {
		if err := os.MkdirAll(dirPath, DefaultDirPerms); err != nil {
			return wrapError("mkdir_all", dirPath, err)
		}
	}

	// Use errgroup for concurrent processing
	g, ctx := errgroup.WithContext(ctx)

	// Set concurrency limit
	scalingFactor := 2
	maxWorkers := max(1, runtime.NumCPU()/scalingFactor)
	g.SetLimit(maxWorkers)

	// Process work items concurrently
	for _, work := range workItems {
		g.Go(func() error {
			switch work.itemType {
			case "link":
				err := content.Link(ctx, work.entry.Hash, work.path)
				if err != nil {
					return wrapError("link_blob", work.path, err)
				}
			case "subtree":
				treeData, err := content.Read(work.entry.Hash)
				if err != nil {
					return wrapError("read_tree", work.entry.Hash, err)
				}

				subTree, err := ParseTree(string(treeData), work.path)
				if err != nil {
					return wrapError("parse_tree", work.entry.Hash, err)
				}

				err = subTree.LinkTree(ctx, store, work.path)
				if err != nil {
					return wrapError("link_subtree", work.path, err)
				}
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and return first error if any
	return g.Wait()
}

package cas

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	maxWorkers := max(1, runtime.NumCPU()/4)
	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	// Process entries concurrently
	for _, entry := range t.entries {
		// Check for cancellation or errors
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-errChan:
			return err
		default:
		}

		entryPath := filepath.Join(targetDir, entry.Path)
		if err := os.MkdirAll(filepath.Dir(entryPath), DefaultDirPerms); err != nil {
			return wrapError("mkdir_all", entryPath, err)
		}

		wg.Add(1)
		go func(entry TreeEntry, path string) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			var err error
			switch entry.Type {
			case "blob":
				err = content.Link(ctx, entry.Hash, path)
				if err != nil {
					err = wrapError("link_blob", path, err)
				}
			case "tree":
				var treeData []byte
				treeData, err = content.Read(entry.Hash)
				if err != nil {
					err = wrapError("read_tree", entry.Hash, err)
					break
				}

				var subTree *Tree
				subTree, err = ParseTree(string(treeData), path)
				if err != nil {
					err = wrapError("parse_tree", entry.Hash, err)
					break
				}

				err = subTree.LinkTree(ctx, store, path)
				if err != nil {
					err = wrapError("link_subtree", path, err)
				}
			}

			if err != nil {
				select {
				case errChan <- err:
				default:
				}
			}
		}(entry, entryPath)
	}

	// Wait for all workers to complete
	wg.Wait()

	// Check for any errors that occurred
	select {
	case err := <-errChan:
		return err
	default:
		return nil
	}
}

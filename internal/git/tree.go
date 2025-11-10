package git

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	// The minimum number of parts in the stdout of the `git ls-tree` command
	minTreePartsLength = 4
)

// Tree represents a git tree object with its entries
type Tree struct {
	entries []TreeEntry
	path    string
	data    []byte
}

// TreeEntry represents a single entry in a git tree
type TreeEntry struct {
	Mode string
	Type string
	Hash string
	Path string
}

// Write writes a tree to a given writer
func (t *Tree) Write(w io.Writer) error {
	for _, entry := range t.entries {
		_, err := fmt.Fprintf(w, "%s %s %s\t%s\n", entry.Mode, entry.Type, entry.Hash, entry.Path)
		if err != nil {
			return err
		}
	}

	return nil
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
		return nil, &WrappedError{
			Op:      "parse_tree",
			Context: "failed to read tree output",
			Err:     err,
		}
	}

	return &Tree{
		entries: entries,
		path:    path,
		data:    []byte(output),
	}, nil
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

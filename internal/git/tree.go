package git

import (
	"fmt"
	"io"
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

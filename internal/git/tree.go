package git

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// newline is hoisted so splitting and counting entries do not allocate a
// separator on every parse.
var newline = []byte{'\n'}

// Tree entry types as printed by `git ls-tree` and carried in
// [TreeEntry.Type].
const (
	// EntryTypeBlob marks a file (or symlink) entry.
	EntryTypeBlob = "blob"
	// EntryTypeTree marks a directory entry.
	EntryTypeTree = "tree"
	// EntryTypeCommit marks a gitlink entry: a submodule pinned to the
	// named commit, whose objects live in another repository.
	EntryTypeCommit = "commit"
)

// GitmodulesPath is the repository-root path of the file registering
// submodule paths and URLs.
const GitmodulesPath = ".gitmodules"

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

// ParseTree parses the stdout of git ls-tree [-r] into a Tree object.
func ParseTree(output []byte, path string) (*Tree, error) {
	entries := make([]TreeEntry, 0, bytes.Count(output, newline)+1)

	for line := range bytes.SplitSeq(output, newline) {
		if len(line) == 0 {
			continue
		}

		entry, err := ParseTreeEntry(string(line))
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return &Tree{
		entries: entries,
		path:    path,
		data:    output,
	}, nil
}

// ParseTreeEntry parses a single "<mode> <type> <hash>\t<path>" line from git
// ls-tree output. A space is accepted in place of the tab so trees written by
// [Tree.Write] round-trip.
func ParseTreeEntry(line string) (TreeEntry, error) {
	mode, rest, ok := strings.Cut(line, " ")
	if !ok {
		return TreeEntry{}, errInvalidTreeEntry()
	}

	typ, rest, ok := strings.Cut(rest, " ")
	if !ok {
		return TreeEntry{}, errInvalidTreeEntry()
	}

	hash, path, ok := cutHashFromPath(rest)
	if !ok || path == "" {
		return TreeEntry{}, errInvalidTreeEntry()
	}

	return TreeEntry{
		Mode: mode,
		Type: typ,
		Hash: hash,
		Path: path,
	}, nil
}

// cutHashFromPath splits a hash from the path that follows it, accepting
// either delimiter git may have used. strings.IndexAny would read more
// naturally but scans a byte at a time, whereas each IndexByte call is
// vectorized, so two passes beat one.
func cutHashFromPath(s string) (hash, path string, found bool) {
	i := strings.IndexByte(s, '\t')
	if j := strings.IndexByte(s, ' '); j >= 0 && (i < 0 || j < i) {
		i = j
	}

	if i < 0 {
		return s, "", false
	}

	return s[:i], s[i+1:], true
}

func errInvalidTreeEntry() error {
	return &WrappedError{
		Op:      "parse_tree_entry",
		Context: "invalid tree entry format",
		Err:     ErrParseTree,
	}
}

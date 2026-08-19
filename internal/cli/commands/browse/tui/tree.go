// Package tui renders the discovered Terragrunt estate as an interactive,
// yazi-style Miller-columns browser for the `terragrunt browse` command.
package tui

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// Kind classifies a tree node for coloring and navigation.
type Kind int

const (
	// KindDir is a plain directory with no component at its path.
	KindDir Kind = iota
	// KindUnit is a discovered unit.
	KindUnit
	// KindStack is a discovered stack.
	KindStack
	// KindFile is a non-component file, shown dimmed for context.
	KindFile
)

// Node is a single entry in the Miller-columns tree: a directory (which may
// also be a unit or stack) or a plain file. Units and stacks are colored by
// kind; files are dimmed.
type Node struct {
	parent       *Node
	component    component.Component
	name         string
	relPath      string
	absPath      string
	preview      string
	children     []*Node
	kind         Kind
	previewWidth int
	othersLoaded bool
	previewReady bool
	previewDark  bool
}

// BuildTree builds the navigable tree from the discovered components.
// workingDir is the browse command's working directory, used to compute
// relative display paths and as the root.
func BuildTree(workingDir string, components component.Components) *Node {
	root := &Node{
		name:    ".",
		relPath: ".",
		absPath: workingDir,
		kind:    KindDir,
	}

	index := map[string]*Node{workingDir: root}

	placeComponents(workingDir, components, root, index)
	sortTree(root)

	return root
}

// Name returns the node's display label: its path segment.
func (n *Node) Name() string { return n.name }

// RelPath returns the node's path relative to the working directory.
func (n *Node) RelPath() string { return n.relPath }

// Kind returns the node's classification.
func (n *Node) Kind() Kind { return n.kind }

// Children returns the node's child entries, sorted alphabetically by name.
func (n *Node) Children() []*Node { return n.children }

// Preview returns the node's rendered file preview, empty until one is built.
func (n *Node) Preview() string { return n.preview }

// NewRoot returns a bare root node for workingDir with no children. The tree
// fills in lazily from the filesystem as directories are visited, and discovery
// annotates units and stacks as it resolves them.
func NewRoot(workingDir string) *Node {
	return BuildTree(workingDir, nil)
}

// ensureDir walks from the root, creating intermediate directory nodes for
// each missing ancestor of absPath, and returns the node at absPath.
func ensureDir(workingDir, absPath string, root *Node, index map[string]*Node) *Node {
	if n, ok := index[absPath]; ok {
		return n
	}

	parentAbs := filepath.Dir(absPath)

	// filepath.Dir is idempotent at the filesystem root, so stop here to avoid
	// infinite recursion when a component lives outside the working dir.
	if parentAbs == absPath {
		return root
	}

	parent := ensureDir(workingDir, parentAbs, root, index)

	n := &Node{
		parent:  parent,
		name:    filepath.Base(absPath),
		relPath: relTo(workingDir, absPath),
		absPath: absPath,
		kind:    KindDir,
	}
	parent.children = append(parent.children, n)
	index[absPath] = n

	return n
}

// placeComponents inserts each component at its path, upgrading any directory
// placeholder created earlier for a nested component.
func placeComponents(workingDir string, components component.Components, root *Node, index map[string]*Node) {
	for _, c := range components {
		abs := c.Path()
		kind := KindUnit

		if c.Kind() == component.StackKind {
			kind = KindStack
		}

		if existing, ok := index[abs]; ok {
			existing.component = c
			existing.kind = kind

			continue
		}

		parent := ensureDir(workingDir, filepath.Dir(abs), root, index)

		n := &Node{
			parent:    parent,
			component: c,
			name:      filepath.Base(abs),
			relPath:   relTo(workingDir, abs),
			absPath:   abs,
			kind:      kind,
		}
		parent.children = append(parent.children, n)
		index[abs] = n
	}
}

// sortTree recursively orders each node's children alphabetically by name.
func sortTree(n *Node) {
	sortChildren(n)

	for _, child := range n.children {
		sortTree(child)
	}
}

// sortChildren orders a single node's children alphabetically by name.
func sortChildren(n *Node) {
	slices.SortFunc(n.children, func(a, b *Node) int {
		return strings.Compare(a.name, b.name)
	})
}

// loadDir reads the directory's filesystem entries and merges any not already
// present into its children, classifying each with a cheap stat: a directory
// containing terragrunt.stack.hcl is a stack, one containing terragrunt.hcl is a
// unit, anything else is a plain directory, and non-directories are files. When
// discovery has already resolved a component for an entry's path, that authority
// is applied instead. It's best-effort: a read error leaves the existing
// children untouched, and loading happens at most once per node.
func (m *Model) loadDir(n *Node) {
	if n.othersLoaded {
		return
	}

	n.othersLoaded = true

	entries, err := vfs.ReadDirEntries(m.fs, n.absPath)
	if err != nil {
		return
	}

	existing := make(map[string]struct{}, len(n.children))
	for _, c := range n.children {
		existing[c.name] = struct{}{}
	}

	for _, entry := range entries {
		name := entry.Name()
		if _, ok := existing[name]; ok {
			continue
		}

		abs := filepath.Join(n.absPath, name)

		child := &Node{
			parent:  n,
			name:    name,
			relPath: filepath.Join(n.relPath, name),
			absPath: abs,
			kind:    m.classify(n, entry, abs),
		}

		if c, ok := m.disc.lookup(abs); ok {
			child.component = c
			child.kind = kindForComponent(c)
		}

		n.children = append(n.children, child)
	}

	sortChildren(n)
}

// classify determines a node's kind from the filesystem alone: directories are
// inspected for a stack or unit config file, and everything else is a file.
// Directories discovery never scans (.git, .terraform, .terragrunt-cache, or
// anything beneath them) stay plain directories even when they hold config
// files, so cache copies don't masquerade as units discovery will never resolve.
func (m *Model) classify(parent *Node, entry fs.DirEntry, abs string) Kind {
	if !entry.IsDir() {
		return KindFile
	}

	if inIgnorableDir(parent, entry.Name()) {
		return KindDir
	}

	switch {
	case m.containsFile(abs, config.DefaultStackFile):
		return KindStack
	case m.containsFile(abs, config.DefaultTerragruntConfigPath):
		return KindUnit
	default:
		return KindDir
	}
}

// inIgnorableDir reports whether name, as a child of parent, is one of the
// directories discovery skips, or lies beneath one. It shares discovery's
// notion of "ignorable" via [util.SkipDirIfIgnorable] so the two layers can't
// drift apart.
func inIgnorableDir(parent *Node, name string) bool {
	if util.SkipDirIfIgnorable(name) != nil {
		return true
	}

	for n := parent; n != nil; n = n.parent {
		if util.SkipDirIfIgnorable(n.name) != nil {
			return true
		}
	}

	return false
}

// containsFile reports whether dir holds a file named name. A stat error means
// the file can't be confirmed, so it counts as absent.
func (m *Model) containsFile(dir, name string) bool {
	exists, err := vfs.FileExists(m.fs, filepath.Join(dir, name))

	return err == nil && exists
}

// kindForComponent maps a discovered component to its tree kind.
func kindForComponent(c component.Component) Kind {
	if c.Kind() == component.StackKind {
		return KindStack
	}

	return KindUnit
}

// relTo returns target relative to base, falling back to target when a relative
// path can't be computed (e.g. paths on different volumes).
func relTo(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return target
	}

	return rel
}

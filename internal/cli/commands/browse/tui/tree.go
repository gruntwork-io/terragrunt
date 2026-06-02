// Package tui renders the discovered Terragrunt estate as an interactive,
// yazi-style Miller-columns browser for the `terragrunt browse` command.
package tui

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

// Node is a single entry in the Miller-columns tree. A Node is a directory,
// which may also be a unit or stack, or a plain file shown dimmed for context.
type Node struct {
	// parent is nil only for the root.
	parent *Node
	// component is non-nil only for unit and stack nodes.
	component component.Component
	// name is the path segment used as the display label.
	name string
	// relPath is the path relative to the working dir, shown in the preview.
	relPath string
	// absPath is the absolute filesystem path this node represents.
	absPath string
	// preview caches the rendered, syntax-highlighted file preview. It's
	// populated lazily and reused while previewWidth and previewDark still match
	// the current pane width and terminal background.
	preview string
	// children are sorted alphabetically by name.
	children []*Node
	kind     Kind
	// previewWidth is the pane interior width preview was rendered for; a resize
	// past it invalidates the cache.
	previewWidth int
	// other marks an entry read from the filesystem rather than discovered as a
	// unit or stack. These are rendered dimmed, for context.
	other bool
	// othersLoaded records that this directory's filesystem entries have already
	// been merged into its children, so we don't read it again.
	othersLoaded bool
	// previewReady records that preview has been rendered for previewWidth and
	// previewDark.
	previewReady bool
	// previewDark is the terminal-background assumption preview was rendered
	// under, so a background change invalidates the cache.
	previewDark bool
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

// Other reports whether the node is a filesystem entry surfaced for context
// rather than a discovered unit or stack.
func (n *Node) Other() bool { return n.other }

// Preview returns the node's rendered file preview, empty until one is built.
func (n *Node) Preview() string { return n.preview }

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
		relPath: relPath(workingDir, absPath),
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
			relPath:   relPath(workingDir, abs),
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

// loadOthers merges the directory's filesystem entries into its children as
// dimmed "other" nodes for context, skipping any path already present as a
// discovered component or ancestor directory. It's best-effort: a read error
// leaves the existing children untouched. Loading happens at most once per node.
func loadOthers(fs vfs.FS, n *Node) {
	if n.othersLoaded {
		return
	}

	n.othersLoaded = true

	entries, err := vfs.ReadDirEntries(fs, n.absPath)
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

		kind := KindFile
		if entry.IsDir() {
			kind = KindDir
		}

		n.children = append(n.children, &Node{
			parent:  n,
			name:    name,
			relPath: filepath.Join(n.relPath, name),
			absPath: filepath.Join(n.absPath, name),
			kind:    kind,
			other:   true,
		})
	}

	sortChildren(n)
}

// counts returns the number of units and stacks among the node's descendants.
func (n *Node) counts() (units, stacks int) {
	for _, c := range n.children {
		switch c.kind {
		case KindUnit:
			units++
		case KindStack:
			stacks++
		case KindDir, KindFile:
		}

		u, s := c.counts()
		units += u
		stacks += s
	}

	return units, stacks
}

// relPath returns absPath relative to workingDir, falling back to absPath when
// a relative path can't be computed.
func relPath(workingDir, absPath string) string {
	rel, err := filepath.Rel(workingDir, absPath)
	if err != nil {
		return absPath
	}

	return rel
}

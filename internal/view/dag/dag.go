// Package dag provides shared DAG tree rendering for displaying component dependency hierarchies.
package dag

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/tree"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/mgutz/ansi"
)

// ListedComponent represents a component for display in a DAG tree.
type ListedComponent struct {
	Type         component.Kind
	Path         string
	Dependencies []*ListedComponent
	Excluded     bool
}

// ListedComponents is a slice of ListedComponent pointers.
type ListedComponents []*ListedComponent

// Contains checks to see if the given path is in the listed components.
func (l ListedComponents) Contains(path string) bool {
	return slices.ContainsFunc(l, func(c *ListedComponent) bool {
		return c.Path == path
	})
}

// Get returns the component with the given path.
func (l ListedComponents) Get(path string) *ListedComponent {
	idx := slices.IndexFunc(l, func(c *ListedComponent) bool {
		return c.Path == path
	})
	if idx == -1 {
		return nil
	}

	return l[idx]
}

// Sort sorts the listed components alphabetically by path.
func (l ListedComponents) Sort() {
	slices.SortFunc(l, func(a, b *ListedComponent) int {
		return strings.Compare(a.Path, b.Path)
	})
}

// Colorizer is a colorizer for the discovered components.
type Colorizer struct {
	unitColorizer    func(string) string
	stackColorizer   func(string) string
	headingColorizer func(string) string
	pathColorizer    func(string) string
}

// NewColorizer creates a new Colorizer.
func NewColorizer(shouldColor bool) *Colorizer {
	if !shouldColor {
		return &Colorizer{
			unitColorizer:    func(s string) string { return s },
			stackColorizer:   func(s string) string { return s },
			headingColorizer: func(s string) string { return s },
			pathColorizer:    func(s string) string { return s },
		}
	}

	return &Colorizer{
		unitColorizer:    ansi.ColorFunc("blue+bh"),
		stackColorizer:   ansi.ColorFunc("green+bh"),
		headingColorizer: ansi.ColorFunc("yellow+bh"),
		pathColorizer:    ansi.ColorFunc("white+d"),
	}
}

// Colorize colors a component's path based on its type.
func (c *Colorizer) Colorize(listedComponent *ListedComponent) string {
	path := listedComponent.Path

	// Get the directory and base name using filepath
	dir, base := filepath.Split(path)

	if dir == "" {
		// No directory part, color the whole path
		switch listedComponent.Type {
		case component.UnitKind:
			return c.unitColorizer(path)
		case component.StackKind:
			return c.stackColorizer(path)
		default:
			return path
		}
	}

	// Color the components differently
	coloredPath := c.pathColorizer(dir)

	switch listedComponent.Type {
	case component.UnitKind:
		return coloredPath + c.unitColorizer(base)
	case component.StackKind:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

// ColorizeType colors a component type label.
func (c *Colorizer) ColorizeType(t component.Kind) string {
	switch t {
	case component.UnitKind:
		// This extra space is to keep unit and stack
		// output equally spaced.
		return c.unitColorizer("unit ")
	case component.StackKind:
		return c.stackColorizer("stack")
	default:
		return string(t)
	}
}

// ColorizeHeading colors a heading string.
func (c *Colorizer) ColorizeHeading(dep string) string {
	return c.headingColorizer(dep)
}

// TreeStyler applies styling to a tree.
type TreeStyler struct {
	entryStyle  lipgloss.Style
	rootStyle   lipgloss.Style
	colorizer   *Colorizer
	shouldColor bool
}

// NewTreeStyler creates a new TreeStyler.
func NewTreeStyler(shouldColor bool) *TreeStyler {
	colorizer := NewColorizer(shouldColor)

	return &TreeStyler{
		shouldColor: shouldColor,
		entryStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")).MarginRight(1),
		rootStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("35")),
		colorizer:   colorizer,
	}
}

// Colorizer returns the underlying colorizer.
func (s *TreeStyler) Colorizer() *Colorizer {
	return s.colorizer
}

// Style applies styling to the given tree.
func (s *TreeStyler) Style(t *tree.Tree) *tree.Tree {
	t = t.
		Enumerator(tree.RoundedEnumerator)

	if !s.shouldColor {
		return t
	}

	return t.
		EnumeratorStyle(s.entryStyle).
		RootStyle(s.rootStyle)
}

// GenerateDAGTree creates a tree structure from ListedComponents.
// It assumes that the components are already sorted in DAG order.
// As such, it will first construct root nodes for each component
// without a dependency in the listed components. Then, it will
// connect the remaining nodes to their dependencies, which
// should be doable in a single pass through the components.
// There may be duplicate entries for dependency nodes, as
// a node may be a dependency for multiple components.
// That's OK.
func GenerateDAGTree(components ListedComponents, s *TreeStyler) *tree.Tree {
	root := tree.Root(".")

	rootNodes := make(map[string]*tree.Tree)
	dependencyNodes := make(map[string]*tree.Tree)
	rootAncestor := make(map[string]string)
	subtreeSize := make(map[string]int)

	// First pass: create all root nodes
	for _, c := range components {
		if len(c.Dependencies) == 0 || !components.Contains(c.Path) {
			rootNodes[c.Path] = tree.New().Root(s.colorizer.Colorize(c))
		}
	}

	// Second pass: connect dependencies
	for _, c := range components {
		if len(c.Dependencies) == 0 {
			continue
		}

		// Sort dependencies to ensure deterministic order
		sortedDeps := make([]string, len(c.Dependencies))
		for i, dep := range c.Dependencies {
			sortedDeps[i] = dep.Path
		}

		sort.Strings(sortedDeps)

		for _, dependency := range sortedDeps {
			if _, exists := rootNodes[dependency]; exists {
				dependencyNode := tree.New().Root(s.colorizer.Colorize(c))
				rootNodes[dependency].Child(dependencyNode)
				dependencyNodes[c.Path] = dependencyNode
				rootAncestor[c.Path] = dependency
				subtreeSize[dependency]++

				continue
			}

			if _, exists := dependencyNodes[dependency]; exists {
				newDependencyNode := tree.New().Root(s.colorizer.Colorize(c))
				dependencyNodes[dependency].Child(newDependencyNode)
				dependencyNodes[c.Path] = newDependencyNode
				ancestor := rootAncestor[dependency]
				rootAncestor[c.Path] = ancestor
				subtreeSize[ancestor]++
			}
		}
	}

	// Sort root nodes: by subtree size ascending (standalone first, deep chains last),
	// then alphabetically as tiebreaker.
	sortedRootPaths := make([]string, 0, len(rootNodes))
	for path := range rootNodes {
		sortedRootPaths = append(sortedRootPaths, path)
	}

	sort.Slice(sortedRootPaths, func(i, j int) bool {
		si, sj := subtreeSize[sortedRootPaths[i]], subtreeSize[sortedRootPaths[j]]
		if si != sj {
			return si < sj
		}

		return sortedRootPaths[i] < sortedRootPaths[j]
	})

	for _, path := range sortedRootPaths {
		root.Child(rootNodes[path])
	}

	return root
}

// FromComponents converts a slice of component.Component into ListedComponents
// suitable for DAG tree rendering. When useDisplayPath is true, DisplayPath() is
// used; otherwise Path() is used.
func FromComponents(components []component.Component, useDisplayPath bool) ListedComponents {
	return fromComponents(components, useDisplayPath, false)
}

// FromComponentsReversed converts a slice of component.Component into ListedComponents
// with the dependency direction inverted: each component's "Dependencies" field is
// populated with its dependents (components that depend on it) instead of its
// dependencies. This causes GenerateDAGTree to render the tree from leaves inward,
// which reflects the execution order for destroy commands.
func FromComponentsReversed(components []component.Component, useDisplayPath bool) ListedComponents {
	return fromComponents(components, useDisplayPath, true)
}

func fromComponents(components []component.Component, useDisplayPath bool, reverse bool) ListedComponents {
	listed := make(ListedComponents, 0, len(components))
	byPath := make(map[string]*ListedComponent, len(components))

	for _, c := range components {
		path := c.Path()
		if useDisplayPath {
			path = c.DisplayPath()
		}

		lc := &ListedComponent{
			Type: c.Kind(),
			Path: path,
		}

		listed = append(listed, lc)
		byPath[c.Path()] = lc
	}

	wireRelationships(components, byPath, reverse)

	return listed
}

func wireRelationships(components []component.Component, byPath map[string]*ListedComponent, reverse bool) {
	for _, c := range components {
		lc := byPath[c.Path()]

		for _, rel := range relatedComponents(c, reverse) {
			if relLC, ok := byPath[rel.Path()]; ok {
				lc.Dependencies = append(lc.Dependencies, relLC)
			}
		}
	}
}

// relatedComponents returns the dependents of a component when reverse is true
// (for destroy order), or its dependencies when false (for apply order).
func relatedComponents(c component.Component, reverse bool) component.Components {
	if reverse {
		return c.Dependents()
	}

	return c.Dependencies()
}

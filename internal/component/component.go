// Package component provides types for representing discovered Terragrunt components.
//
// These include units and stacks.
//
// This package contains only data types and their associated methods, with no discovery logic.
// It exists separately from the discovery package to allow other packages (like filter) to
// depend on these types without creating circular dependencies.
package component

import (
	"slices"
	"sort"
	"sync"

	"errors"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// Kind is the type of Terragrunt component.
type Kind string

// Component represents a discovered Terragrunt configuration.
// This interface is implemented by Unit and Stack.
type Component interface {
	Kind() Kind
	Path() string
	SetPath(string)
	DisplayPath() string
	External() bool
	SetExternal()
	Reading() []string
	SetReading(...string)
	Sources() []string
	ConfigFile() string
	DiscoveryContext() *DiscoveryContext
	SetDiscoveryContext(*DiscoveryContext)
	Origin() Origin
	AddDependency(Component)
	AddDependent(Component)
	Dependencies() Components
	Dependents() Components

	lock()
	unlock()

	ensureDependency(Component)
	ensureDependent(Component)
}

// Origin determines the discovery origin of a component.
// This is important if there are multiple different reasons that a component might have been discovered.
//
// e.g. A component might be discovered in a Git worktree due to graph discovery from the results of a Git-based filter.
type Origin string

const (
	OriginUnknown               Origin = "unknown"
	OriginWorktreeDiscovery     Origin = "worktree-discovery"
	OriginGraphDiscovery        Origin = "graph-discovery"
	OriginPathDiscovery         Origin = "path-discovery"
	OriginRelationshipDiscovery Origin = "relationship-discovery"
)

// DiscoveryContext is the context in which
// a Component was discovered.
//
// It's useful to know this information,
// because it can help us determine how the
// Component should be run or enqueued later.
type DiscoveryContext struct {
	WorkingDir string
	Ref        string

	origin Origin

	Cmd  string
	Args []string
}

// Copy returns a copy of the DiscoveryContext.
// Args is deep-copied so mutations on the copy (e.g. slices.DeleteFunc) do not
// corrupt the original's backing array.
func (dc *DiscoveryContext) Copy() *DiscoveryContext {
	c := *dc
	c.Args = slices.Clone(dc.Args)

	return &c
}

// CopyWithNewOrigin returns a copy of the DiscoveryContext with the origin set to the given origin.
//
// Discovered components should never have their origin overridden by subsequent phases of discovery. Only use this
// method if you are discovering a new component that was originally discovered by a different discovery phase.
//
// e.g. A component discovered as a dependency/dependent of a component discovered via Git discovery should be
// considered discovered via graph discovery, not Git discovery.
func (dc *DiscoveryContext) CopyWithNewOrigin(origin Origin) *DiscoveryContext {
	c := dc.Copy()
	c.origin = origin

	return c
}

// Origin returns the origin of the DiscoveryContext.
func (dc *DiscoveryContext) Origin() Origin {
	if dc.origin == "" {
		return OriginUnknown
	}

	return dc.origin
}

// SuggestOrigin suggests an origin for the DiscoveryContext.
//
// Only actually updates the origin if it is empty. This is to ensure that the origin of a component is always
// considered the first origin discovered for that component, and that it can't be overridden by subsequent phases
// of discovery that might re-discover the same component.
func (dc *DiscoveryContext) SuggestOrigin(origin Origin) {
	if dc.origin == "" {
		dc.origin = origin
	}
}

// Components is a list of discovered Terragrunt components.
type Components []Component

// Sort sorts the Components by path.
func (c Components) Sort() Components {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Path() < c[j].Path()
	})

	return c
}

// Filter filters the Components by config type.
func (c Components) Filter(kind Kind) Components {
	if len(c) == 0 {
		return c
	}

	filtered := make(Components, 0, len(c))

	for _, component := range c {
		if component.Kind() == kind {
			filtered = append(filtered, component)
		}
	}

	return filtered
}

// FilterByPath filters the Components by path.
func (c Components) FilterByPath(path string) Components {
	filtered := make(Components, 0, 1)

	for _, component := range c {
		if component.Path() == path {
			filtered = append(filtered, component)
		}
	}

	return filtered
}

// RemoveByPath removes the Component with the given path from the Components.
func (c Components) RemoveByPath(path string) Components {
	if len(c) == 0 {
		return c
	}

	filtered := make(Components, 0, len(c)-1)

	for _, component := range c {
		if component.Path() != path {
			filtered = append(filtered, component)
		}
	}

	return filtered
}

// Paths returns the paths of the Components.
func (c Components) Paths() []string {
	paths := make([]string, 0, len(c))
	for _, component := range c {
		// Skip units explicitly marked as excluded.
		if unit, ok := component.(*Unit); ok && unit.Excluded() {
			continue
		}

		paths = append(paths, component.Path())
	}

	return paths
}

// CycleCheck checks for cycles in the dependency graph.
// If a cycle is detected, it returns the first Component that is part of the cycle, and an error.
// If no cycle is detected, it returns nil and nil.
func (c Components) CycleCheck() (Component, error) {
	visited := make(map[string]bool)
	inPath := make(map[string]bool)

	var checkCycle func(component Component) error

	checkCycle = func(component Component) error {
		if inPath[component.Path()] {
			return errors.New("cycle detected in dependency graph at path: " + component.Path())
		}

		if visited[component.Path()] {
			return nil
		}

		visited[component.Path()] = true
		inPath[component.Path()] = true

		for _, dep := range component.Dependencies() {
			if err := checkCycle(dep); err != nil {
				return err
			}
		}

		inPath[component.Path()] = false

		return nil
	}

	for _, component := range c {
		if !visited[component.Path()] {
			if err := checkCycle(component); err != nil {
				return component, err
			}
		}
	}

	return nil, nil
}

// ThreadSafeComponents provides thread-safe access to a Components slice.
// It compares components by symlink-resolved path, so two spellings of one
// location name the same component.
type ThreadSafeComponents struct {
	paths      *vfs.PathResolver
	byResolved map[string]Component
	components Components
	mu         sync.RWMutex
}

// NewThreadSafeComponents returns a ThreadSafeComponents holding components,
// with paths resolved through paths. When two components resolve to one path,
// lookups return the first.
func NewThreadSafeComponents(paths *vfs.PathResolver, components Components) *ThreadSafeComponents {
	tsc := &ThreadSafeComponents{
		paths:      paths,
		byResolved: make(map[string]Component, len(components)),
		components: components,
	}

	for _, c := range components {
		resolved := paths.Resolve(c.Path())
		if _, ok := tsc.byResolved[resolved]; !ok {
			tsc.byResolved[resolved] = c
		}
	}

	return tsc
}

// EnsureComponent adds c unless a component with the same resolved path is
// present. It returns the component stored at that path and whether c was
// added. When goroutines race to add one path, exactly one adds it and the
// rest get its component.
func (tsc *ThreadSafeComponents) EnsureComponent(c Component) (Component, bool) {
	resolved := tsc.paths.Resolve(c.Path())

	if found := tsc.findResolved(resolved); found != nil {
		return found, false
	}

	return tsc.addComponent(resolved, c)
}

// FindByPath returns the component whose resolved path matches that of path,
// or nil.
func (tsc *ThreadSafeComponents) FindByPath(path string) Component {
	return tsc.findResolved(tsc.paths.Resolve(path))
}

// ToComponents returns a copy of the components slice.
func (tsc *ThreadSafeComponents) ToComponents() Components {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(Components, len(tsc.components))
	copy(result, tsc.components)

	return result
}

// Len returns the number of components in the components slice.
func (tsc *ThreadSafeComponents) Len() int {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	return len(tsc.components)
}

// findResolved returns the component whose path resolves to resolved, or nil.
func (tsc *ThreadSafeComponents) findResolved(resolved string) Component {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	return tsc.byResolved[resolved]
}

// addComponent stores c under resolved and returns it with true. When a
// component is already stored there, it returns that component with false;
// another goroutine may have added it since the caller's lookup.
func (tsc *ThreadSafeComponents) addComponent(resolved string, c Component) (Component, bool) {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	if found, ok := tsc.byResolved[resolved]; ok {
		return found, false
	}

	tsc.byResolved[resolved] = c
	tsc.components = append(tsc.components, c)

	return c, true
}

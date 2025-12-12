// Package component provides types for representing discovered Terragrunt components.
//
// These include units and stacks.
//
// This package contains only data types and their associated methods, with no discovery logic.
// It exists separately from the discovery package to allow other packages (like filter) to
// depend on these types without creating circular dependencies.
package component

import (
	"path/filepath"
	"slices"
	"sort"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// resolvePath resolves symlinks in a path for consistent comparison across platforms.
// On macOS, /var is a symlink to /private/var, so paths must be resolved.
func resolvePath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}

	return resolved
}

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
	DiscoveryContext() *DiscoveryContext
	SetDiscoveryContext(*DiscoveryContext)
	AddDependency(Component)
	AddDependent(Component)
	Dependencies() Components
	Dependents() Components

	lock()
	unlock()
	rLock()
	rUnlock()

	ensureDependency(Component)
	ensureDependent(Component)
}

// DiscoveryContext is the context in which
// a Component was discovered.
//
// It's useful to know this information,
// because it can help us determine how the
// Component should be run or enqueued later.
type DiscoveryContext struct {
	WorkingDir string
	Ref        string

	Cmd  string
	Args []string
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
// It uses an RWMutex to allow concurrent reads and serialized writes.
// Resolved paths are cached to avoid repeated filepath.EvalSymlinks syscalls
// and ensure consistent symlink-aware comparisons across all methods.
type ThreadSafeComponents struct {
	resolvedPaths map[string]string
	components    Components
	mu            sync.RWMutex
}

// NewThreadSafeComponents creates a new ThreadSafeComponents instance with the given components.
func NewThreadSafeComponents(components Components) *ThreadSafeComponents {
	tsc := &ThreadSafeComponents{
		components:    components,
		resolvedPaths: make(map[string]string, len(components)),
	}

	// Pre-populate resolved paths cache for initial components
	for _, c := range components {
		tsc.resolvedPaths[c.Path()] = resolvePath(c.Path())
	}

	return tsc
}

// resolvedPathFor returns the cached resolved path for a component path if present,
// otherwise resolves the path on the fly without mutating the cache.
// Caller must hold at least a read lock.
func (tsc *ThreadSafeComponents) resolvedPathFor(path string) string {
	if resolved, ok := tsc.resolvedPaths[path]; ok {
		return resolved
	}

	return resolvePath(path)
}

// EnsureComponent adds a component to the components list if it's not already present.
// This method is TOCTOU-safe (Time-Of-Check-Time-Of-Use) by using a double-check pattern.
// Path comparison uses resolved symlink paths for consistency.
//
// It returns the component if it was added, and a boolean indicating if it was added.
func (tsc *ThreadSafeComponents) EnsureComponent(c Component) (Component, bool) {
	found, ok := tsc.findComponent(c)
	if !ok {
		return tsc.addComponent(c)
	}

	return found, false
}

// findComponent checks if a component is in the components slice using resolved paths.
// If it is, it returns the component and true.
// If it is not, it returns nil and false.
func (tsc *ThreadSafeComponents) findComponent(c Component) (Component, bool) {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	searchResolved := resolvePath(c.Path())

	idx := slices.IndexFunc(tsc.components, func(cc Component) bool {
		return tsc.resolvedPathFor(cc.Path()) == searchResolved
	})

	if idx == -1 {
		return nil, false
	}

	return tsc.components[idx], true
}

// addComponent adds a component to the components list, acquiring a write lock.
// Uses a double-check pattern to avoid TOCTOU race conditions.
// Caches the resolved path for the new component.
func (tsc *ThreadSafeComponents) addComponent(c Component) (Component, bool) {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	searchResolved := resolvePath(c.Path())

	// Do one last check to see if the component is already in the components list
	// to avoid a TOCTOU race condition. Uses resolved paths for comparison.
	idx := slices.IndexFunc(tsc.components, func(cc Component) bool {
		return tsc.resolvedPathFor(cc.Path()) == searchResolved
	})

	if idx != -1 {
		return tsc.components[idx], false
	}

	// Cache resolved path and add component
	tsc.resolvedPaths[c.Path()] = searchResolved
	tsc.components = append(tsc.components, c)

	return c, true
}

// FindByPath searches for a component by its path and returns it if found, otherwise returns nil.
// Paths are resolved to handle symlinks consistently across platforms (e.g., macOS /var -> /private/var).
// Uses cached resolved paths to avoid repeated syscalls.
func (tsc *ThreadSafeComponents) FindByPath(path string) Component {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	resolvedSearchPath := resolvePath(path)

	for _, c := range tsc.components {
		if tsc.resolvedPathFor(c.Path()) == resolvedSearchPath {
			return c
		}
	}

	return nil
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

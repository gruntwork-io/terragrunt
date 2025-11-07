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

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// Kind is the type of Terragrunt component.
type Kind string

// Component represents a discovered Terragrunt configuration.
// This interface is implemented by Unit and Stack.
type Component interface {
	Kind() Kind
	Path() string
	SetPath(string)
	External() bool
	SetExternal()
	Reading() []string
	SetReading(...string)
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
type ThreadSafeComponents struct {
	components Components
	mu         sync.RWMutex
}

// NewThreadSafeComponents creates a new ThreadSafeComponents instance with the given components.
func NewThreadSafeComponents(components Components) *ThreadSafeComponents {
	return &ThreadSafeComponents{
		components: components,
	}
}

// EnsureComponent adds a component to the components list if it's not already present.
// This method is TOCTOU-safe (Time-Of-Check-Time-Of-Use) by using a double-check pattern.
//
// It returns the component if it was added, and a boolean indicating if it was added.
func (tsc *ThreadSafeComponents) EnsureComponent(c Component) (Component, bool) {
	found, ok := tsc.findComponent(c)
	if !ok {
		return tsc.addComponent(c)
	}

	return found, false
}

// findComponent checks if a component is in the components slice.
// If it is, it returns the component and true.
// If it is not, it returns nil and false.
func (tsc *ThreadSafeComponents) findComponent(c Component) (Component, bool) {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	idx := slices.IndexFunc(tsc.components, func(cc Component) bool {
		return cc.Path() == c.Path()
	})

	if idx == -1 {
		return nil, false
	}

	return tsc.components[idx], true
}

// addComponent adds a component to the components list, acquiring a write lock.
// Uses a double-check pattern to avoid TOCTOU race conditions.
func (tsc *ThreadSafeComponents) addComponent(c Component) (Component, bool) {
	tsc.mu.Lock()
	defer tsc.mu.Unlock()

	// Do one last check to see if the component is already in the components list
	// to avoid a TOCTOU race condition.
	idx := slices.IndexFunc(tsc.components, func(cc Component) bool {
		return cc.Path() == c.Path()
	})

	if idx != -1 {
		return tsc.components[idx], false
	}

	tsc.components = append(tsc.components, c)

	return c, true
}

// FindByPath searches for a component by its path and returns it if found, otherwise returns nil.
func (tsc *ThreadSafeComponents) FindByPath(path string) Component {
	tsc.mu.RLock()
	defer tsc.mu.RUnlock()

	for _, c := range tsc.components {
		if c.Path() == path {
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

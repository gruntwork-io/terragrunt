// Package component provides types for representing discovered Terragrunt components.
//
// These include units and stacks.
//
// This package contains only data types and their associated methods, with no discovery logic.
// It exists separately from the discovery package to allow other packages (like filter) to
// depend on these types without creating circular dependencies.
package component

import (
	"sort"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

const (
	// Unit is a component of kind unit.
	Unit Kind = "unit"
	// Stack is a component of kind stack.
	Stack Kind = "stack"
)

// Kind is the type of Terragrunt component.
type Kind string

// Component represents a discovered Terragrunt configuration.
type Component struct {
	Parsed           *config.TerragruntConfig
	DiscoveryContext *DiscoveryContext

	Kind Kind
	Path string

	dependencies Components
	dependents   Components

	External bool
}

// AddDependency adds a dependency to the Component and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (c *Component) AddDependency(dependency *Component) {
	c.dependencies = append(c.dependencies, dependency)

	dependency.dependents = append(dependency.dependents, c)

}

// AddDependent adds a dependent to the Component and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (c *Component) AddDependent(dependent *Component) {
	c.dependents = append(c.dependents, dependent)

	dependent.dependencies = append(dependent.dependencies, c)
}

// Dependencies returns the dependencies of the Component.
func (c *Component) Dependencies() Components {
	return c.dependencies
}

// Dependents returns the dependents of the Component.
func (c *Component) Dependents() Components {
	return c.dependents
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
type Components []*Component

// Sort sorts the Components by path.
func (c Components) Sort() Components {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Path < c[j].Path
	})

	return c
}

// Filter filters the Components by config type.
func (c Components) Filter(kind Kind) Components {
	filtered := make(Components, 0, len(c))

	for _, component := range c {
		if component.Kind == kind {
			filtered = append(filtered, component)
		}
	}

	return filtered
}

// FilterByPath filters the Components by path.
func (c Components) FilterByPath(path string) Components {
	filtered := make(Components, 0, 1)

	for _, component := range c {
		if component.Path == path {
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
		if component.Path != path {
			filtered = append(filtered, component)
		}
	}

	return filtered
}

// Paths returns the paths of the Components.
func (c Components) Paths() []string {
	paths := make([]string, 0, len(c))
	for _, component := range c {
		paths = append(paths, component.Path)
	}

	return paths
}

// CycleCheck checks for cycles in the dependency graph.
// If a cycle is detected, it returns the first Component that is part of the cycle, and an error.
// If no cycle is detected, it returns nil and nil.
func (c Components) CycleCheck() (*Component, error) {
	visited := make(map[string]bool)
	inPath := make(map[string]bool)

	var checkCycle func(component *Component) error

	checkCycle = func(component *Component) error {
		if inPath[component.Path] {
			return errors.New("cycle detected in dependency graph at path: " + component.Path)
		}

		if visited[component.Path] {
			return nil
		}

		visited[component.Path] = true
		inPath[component.Path] = true

		for _, dep := range component.Dependencies() {
			if err := checkCycle(dep); err != nil {
				return err
			}
		}

		inPath[component.Path] = false

		return nil
	}

	for _, component := range c {
		if !visited[component.Path] {
			if err := checkCycle(component); err != nil {
				return component, err
			}
		}
	}

	return nil, nil
}

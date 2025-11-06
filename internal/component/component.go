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

// Kind is the type of Terragrunt component.
type Kind string

const (
	UnitKind  Kind = "unit"
	StackKind Kind = "stack"
)

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
}

// Unit represents a discovered Terragrunt unit configuration.
type Unit struct {
	cfg              *config.TerragruntConfig
	path             string
	reading          []string
	discoveryContext *DiscoveryContext
	dependencies     Components
	dependents       Components
	external         bool
}

// NewUnit creates a new Unit component with the given path.
func NewUnit(path string) *Unit {
	return &Unit{
		path:         path,
		dependencies: make(Components, 0),
		dependents:   make(Components, 0),
	}
}

// WithReading appends a file to the list of files being read by this component.
// Useful for constructing components with all files read at once.
func (u *Unit) WithReading(files ...string) *Unit {
	u.SetReading(files...)

	return u
}

// WithConfig adds configuration to a Unit component.
func (u *Unit) WithConfig(cfg *config.TerragruntConfig) *Unit {
	u.cfg = cfg

	return u
}

// Config returns the parsed Terragrunt configuration for this unit.
func (u *Unit) Config() *config.TerragruntConfig {
	return u.cfg
}

// StoreConfig stores the parsed Terragrunt configuration for this unit.
func (u *Unit) StoreConfig(cfg *config.TerragruntConfig) {
	u.cfg = cfg
}

// Kind returns the kind of component (always Unit for Unit).
func (u *Unit) Kind() Kind {
	return UnitKind
}

// Path returns the path to the component.
func (u *Unit) Path() string {
	return u.path
}

// SetPath sets the path to the component.
func (u *Unit) SetPath(path string) {
	u.path = path
}

// External returns whether the component is external.
func (u *Unit) External() bool {
	return u.external
}

// SetExternal marks the component as external.
func (u *Unit) SetExternal() {
	u.external = true
}

// Reading returns the list of files being read by this component.
func (u *Unit) Reading() []string {
	return u.reading
}

// SetReading sets the list of files being read by this component.
func (u *Unit) SetReading(files ...string) {
	u.reading = files
}

// DiscoveryContext returns the discovery context for this component.
func (u *Unit) DiscoveryContext() *DiscoveryContext {
	return u.discoveryContext
}

// SetDiscoveryContext sets the discovery context for this component.
func (u *Unit) SetDiscoveryContext(ctx *DiscoveryContext) {
	u.discoveryContext = ctx
}

// AddDependency adds a dependency to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependency(dependency Component) {
	u.dependencies = append(u.dependencies, dependency)

	switch dependency := dependency.(type) {
	case *Unit:
		dependency.dependents = append(dependency.dependents, u)
	case *Stack:
		dependency.dependents = append(dependency.dependents, u)
	}
}

// AddDependent adds a dependent to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependent(dependent Component) {
	u.dependents = append(u.dependents, dependent)

	switch dependent := dependent.(type) {
	case *Unit:
		dependent.dependencies = append(dependent.dependencies, u)
	case *Stack:
		dependent.dependencies = append(dependent.dependencies, u)
	}
}

// Dependencies returns the dependencies of the Unit.
func (u *Unit) Dependencies() Components {
	return u.dependencies
}

// Dependents returns the dependents of the Unit.
func (u *Unit) Dependents() Components {
	return u.dependents
}

// Stack represents a discovered Terragrunt stack configuration.
type Stack struct {
	cfg              *config.StackConfig
	path             string
	reading          []string
	discoveryContext *DiscoveryContext
	dependencies     Components
	dependents       Components
	external         bool
}

// NewStack creates a new Stack component with the given path.
func NewStack(path string) *Stack {
	return &Stack{
		path:         path,
		dependencies: make(Components, 0),
		dependents:   make(Components, 0),
	}
}

// NewStackWithConfig creates a new Stack component with the given path and config.
func NewStackWithConfig(path string, cfg *config.StackConfig) *Stack {
	return &Stack{
		cfg:          cfg,
		path:         path,
		dependencies: make(Components, 0),
		dependents:   make(Components, 0),
	}
}

// Config returns the parsed Stack configuration for this stack.
func (s *Stack) Config() *config.StackConfig {
	return s.cfg
}

// StoreConfig stores the parsed Stack configuration for this stack.
func (s *Stack) StoreConfig(cfg *config.StackConfig) {
	s.cfg = cfg
}

// Kind returns the kind of component (always Stack for Stack).
func (s *Stack) Kind() Kind {
	return StackKind
}

// Path returns the path to the component.
func (s *Stack) Path() string {
	return s.path
}

// SetPath sets the path to the component.
func (s *Stack) SetPath(path string) {
	s.path = path
}

// External returns whether the component is external.
func (s *Stack) External() bool {
	return s.external
}

// SetExternal marks the component as external.
func (s *Stack) SetExternal() {
	s.external = true
}

// Reading returns the list of files being read by this component.
func (s *Stack) Reading() []string {
	return s.reading
}

// SetReading sets the list of files being read by this component.
func (s *Stack) SetReading(files ...string) {
	s.reading = files
}

// DiscoveryContext returns the discovery context for this component.
func (s *Stack) DiscoveryContext() *DiscoveryContext {
	return s.discoveryContext
}

// SetDiscoveryContext sets the discovery context for this component.
func (s *Stack) SetDiscoveryContext(ctx *DiscoveryContext) {
	s.discoveryContext = ctx
}

// AddDependency adds a dependency to the Stack and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (s *Stack) AddDependency(dependency Component) {
	s.dependencies = append(s.dependencies, dependency)
	dependency.AddDependent(s)
}

// AddDependent adds a dependent to the Stack and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (s *Stack) AddDependent(dependent Component) {
	s.dependents = append(s.dependents, dependent)
	dependent.AddDependency(s)
}

// Dependencies returns the dependencies of the Stack.
func (s *Stack) Dependencies() Components {
	return s.dependencies
}

// Dependents returns the dependents of the Stack.
func (s *Stack) Dependents() Components {
	return s.dependents
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

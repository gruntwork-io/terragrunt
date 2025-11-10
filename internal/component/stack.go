package component

import (
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
)

const (
	StackKind Kind = "stack"
)

// Stack represents a discovered Terragrunt stack configuration.
type Stack struct {
	cfg              *config.StackConfig
	path             string
	reading          []string
	discoveryContext *DiscoveryContext
	dependencies     Components
	dependents       Components
	external         bool
	mu               sync.RWMutex
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

// lock locks the Stack.
func (s *Stack) lock() {
	s.mu.Lock()
}

// unlock unlocks the Stack.
func (s *Stack) unlock() {
	s.mu.Unlock()
}

// rLock locks the Stack for reading.
func (s *Stack) rLock() {
	s.mu.RLock()
}

// rUnlock unlocks the Stack for reading.
func (s *Stack) rUnlock() {
	s.mu.RUnlock()
}

// AddDependency adds a dependency to the Stack and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (s *Stack) AddDependency(dependency Component) {
	s.ensureDependency(dependency)

	dependency.ensureDependent(s)
}

// ensureDependency adds a dependency to a stack if it's not already present.
func (s *Stack) ensureDependency(dependency Component) {
	s.lock()
	defer s.unlock()

	if !slices.Contains(s.dependencies, dependency) {
		s.dependencies = append(s.dependencies, dependency)
	}
}

// ensureDependent adds a dependent to a stack if it's not already present.
func (s *Stack) ensureDependent(dependent Component) {
	s.lock()
	defer s.unlock()

	if !slices.Contains(s.dependents, dependent) {
		s.dependents = append(s.dependents, dependent)
	}
}

// AddDependent adds a dependent to the Stack and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (s *Stack) AddDependent(dependent Component) {
	s.ensureDependent(dependent)

	dependent.ensureDependency(s)
}

// Dependencies returns the dependencies of the Stack.
func (s *Stack) Dependencies() Components {
	s.rLock()
	defer s.rUnlock()

	return s.dependencies
}

// Dependents returns the dependents of the Stack.
func (s *Stack) Dependents() Components {
	s.rLock()
	defer s.rUnlock()

	return s.dependents
}

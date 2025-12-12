package component

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	StackKind Kind = "stack"
)

// StackExecution holds execution-specific fields for running a stack.
// This is nil during discovery phase and populated when preparing for execution.
type StackExecution struct {
	Report            *report.Report
	TerragruntOptions *options.TerragruntOptions
}

// Stack represents a discovered Terragrunt stack configuration.
type Stack struct {
	cfg              *config.StackConfig
	discoveryContext *DiscoveryContext
	Execution        *StackExecution
	path             string
	reading          []string
	dependencies     Components
	dependents       Components
	Units            []*Unit
	mu               sync.RWMutex
	external         bool
}

// NewStack creates a new Stack component with the given path.
func NewStack(path string) *Stack {
	return &Stack{
		path:             path,
		discoveryContext: &DiscoveryContext{},
		dependencies:     make(Components, 0),
		dependents:       make(Components, 0),
	}
}

// WithDiscoveryContext sets the discovery context for this stack.
func (s *Stack) WithDiscoveryContext(ctx *DiscoveryContext) *Stack {
	s.discoveryContext = ctx

	return s
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

// DisplayPath returns the path relative to DiscoveryContext.WorkingDir for display purposes.
// Falls back to the original path if relative path calculation fails or WorkingDir is empty.
func (s *Stack) DisplayPath() string {
	if s.discoveryContext == nil || s.discoveryContext.WorkingDir == "" {
		return s.path
	}

	if rel, err := filepath.Rel(s.discoveryContext.WorkingDir, s.path); err == nil {
		return rel
	}

	return s.path
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

// Sources returns the list of sources for this component.
//
// Stacks don't support leveraging sources right now, so we just return an empty list.
func (s *Stack) Sources() []string {
	return []string{}
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

// String renders this stack as a human-readable string.
//
// Example output:
//
//	Stack at /path/to/stack:
//	  => Unit /path/to/unit1 (excluded: false, assume applied: false, dependencies: [/dep1])
//	  => Unit /path/to/unit2 (excluded: true, assume applied: false, dependencies: [])
func (s *Stack) String() string {
	units := make([]string, 0, len(s.Units))
	for _, unit := range s.Units {
		units = append(units, "  => "+unit.String())
	}

	sort.Strings(units)

	workingDir := s.path
	if s.Execution != nil && s.Execution.TerragruntOptions != nil {
		workingDir = s.Execution.TerragruntOptions.WorkingDir
	}

	return fmt.Sprintf("Stack at %s:\n%s", workingDir, strings.Join(units, "\n"))
}

// FindUnitByPath finds a unit in the stack by its path.
func (s *Stack) FindUnitByPath(path string) *Unit {
	for _, unit := range s.Units {
		if unit.Path() == path {
			return unit
		}
	}

	return nil
}

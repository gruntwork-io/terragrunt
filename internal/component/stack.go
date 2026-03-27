package component

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/config"
)

const (
	StackKind Kind = "stack"
)

// Stack represents a discovered Terragrunt stack configuration.
type Stack struct {
	cfg   *config.StackConfig
	Units []*Unit
	baseComponent
}

// NewStack creates a new Stack component with the given path.
func NewStack(path string) *Stack {
	return &Stack{
		baseComponent: newBaseComponent(path),
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

// Sources returns the list of sources for this component.
//
// Stacks don't support leveraging sources right now, so we just return an empty list.
func (s *Stack) Sources() []string {
	return []string{}
}

// ConfigFile returns the config filename for this stack.
func (s *Stack) ConfigFile() string {
	return config.DefaultStackFile
}

// AddDependency adds a dependency to the Stack and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (s *Stack) AddDependency(dependency Component) {
	s.baseComponent.addDependency(s, dependency)
}

// AddDependent adds a dependent to the Stack and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (s *Stack) AddDependent(dependent Component) {
	s.baseComponent.addDependent(s, dependent)
}

// String renders this stack as a human-readable string.
//
// Example output:
//
//	Stack at /path/to/stack:
//	  => Unit /path/to/unit1 (excluded: false, dependencies: [/dep1])
//	  => Unit /path/to/unit2 (excluded: true, dependencies: [])
func (s *Stack) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	units := make([]string, 0, len(s.Units))
	for _, unit := range s.Units {
		units = append(units, "  => "+unit.String())
	}

	sort.Strings(units)

	workingDir := s.path
	if s.discoveryContext != nil && s.discoveryContext.WorkingDir != "" {
		workingDir = s.discoveryContext.WorkingDir
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

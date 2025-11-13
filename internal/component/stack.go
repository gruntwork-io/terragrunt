package component

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	StackKind Kind = "stack"
)

// Stack represents a discovered Terragrunt stack configuration.
// This type serves as a DTO for data exchange between discovery and runner packages.
type Stack struct {
	// Discovery fields (populated by discovery package)
	cfg              *config.StackConfig
	path             string
	reading          []string
	discoveryContext *DiscoveryContext
	dependencies     Components
	dependents       Components
	external         bool

	// Runtime/Execution fields (populated by runner package)
	terragruntOptions *options.TerragruntOptions
	logger            log.Logger
	flagExcluded      bool

	// Thread-safety
	mu sync.RWMutex
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

// TerragruntOptions returns the Terragrunt options for this stack.
func (s *Stack) TerragruntOptions() *options.TerragruntOptions {
	s.rLock()
	defer s.rUnlock()

	return s.terragruntOptions
}

// SetTerragruntOptions sets the Terragrunt options for this stack.
func (s *Stack) SetTerragruntOptions(opts *options.TerragruntOptions) {
	s.lock()
	defer s.unlock()

	s.terragruntOptions = opts
}

// Logger returns the logger for this stack.
func (s *Stack) Logger() log.Logger {
	s.rLock()
	defer s.rUnlock()

	return s.logger
}

// SetLogger sets the logger for this stack.
func (s *Stack) SetLogger(logger log.Logger) {
	s.lock()
	defer s.unlock()

	s.logger = logger
}

// FlagExcluded returns whether this stack was excluded by filters/flags.
func (s *Stack) FlagExcluded() bool {
	s.rLock()
	defer s.rUnlock()

	return s.flagExcluded
}

// SetFlagExcluded sets whether this stack was excluded by filters/flags.
func (s *Stack) SetFlagExcluded(excluded bool) {
	s.lock()
	defer s.unlock()

	s.flagExcluded = excluded
}

// String renders this stack as a human-readable string.
func (s *Stack) String() string {
	s.rLock()
	defer s.rUnlock()

	var dependencies = make([]string, 0, len(s.dependencies))
	for _, dependency := range s.dependencies {
		dependencies = append(dependencies, dependency.Path())
	}

	return fmt.Sprintf(
		"Stack %s (excluded: %v, dependencies: [%s])",
		s.path, s.flagExcluded, strings.Join(dependencies, ", "),
	)
}

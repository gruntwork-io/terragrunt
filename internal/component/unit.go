package component

import (
	"slices"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	UnitKind Kind = "unit"
)

// Unit represents a discovered Terragrunt unit configuration.
type Unit struct {
	cfg              *config.TerragruntConfig
	discoveryContext *DiscoveryContext
	opts             *options.TerragruntOptions
	path             string
	filename         string
	reading          []string
	dependencies     Components
	dependents       Components
	mu               sync.RWMutex
	external         bool
	applyExternal    bool
	filterExcluded   bool
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

// WithOpts adds options to a Unit component.
func (u *Unit) WithOpts(opts *options.TerragruntOptions) *Unit {
	u.opts = opts

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

// Excluded returns whether the unit is excluded.
func (u *Unit) Excluded() bool {
	u.rLock()
	defer u.rUnlock()

	// Filter-based exclusion takes precedence
	if u.filterExcluded {
		return true
	}

	if u.cfg == nil {
		return false
	}

	if u.cfg.Exclude == nil {
		return false
	}

	if u.discoveryContext == nil {
		return false
	}

	if u.isDestroyCommand() && u.isProtectedByPreventDestroy() {
		return true
	}

	return u.cfg.Exclude.IsActionListed(u.discoveryContext.Cmd)
}

// Filename returns the filename of the unit.
func (u *Unit) Filename() string {
	return u.filename
}

// SetFilename sets the filename of the unit.
func (u *Unit) SetFilename(filename string) {
	u.filename = filename
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

// ShouldApplyExternal returns whether an external dependency should be applied.
// For non-external components, this always returns true.
// For external components, it returns the value set via SetShouldApplyExternal.
func (u *Unit) ShouldApplyExternal() bool {
	u.rLock()
	defer u.rUnlock()

	// Non-external components should always be applied
	if !u.external {
		return true
	}

	// For external components, return the stored value (defaults to false if not set)
	return u.applyExternal
}

// SetShouldApplyExternal sets whether an external dependency should be applied.
// This only has effect for external components.
func (u *Unit) SetShouldApplyExternal() {
	u.lock()
	defer u.unlock()

	u.applyExternal = true
}

// SetFilterExcluded sets whether the unit is excluded by a filter.
// This is used by unit filters to mark units as excluded based on filtering logic.
func (u *Unit) SetFilterExcluded(excluded bool) {
	u.lock()
	defer u.unlock()

	u.filterExcluded = excluded
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

// Opts returns the Terragrunt options for this unit.
func (u *Unit) Opts() *options.TerragruntOptions {
	return u.opts
}

// SetOpts sets the Terragrunt options for this unit.
func (u *Unit) SetOpts(opts *options.TerragruntOptions) {
	u.opts = opts
}

// AddDependency adds a dependency to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependency(dependency Component) {
	u.ensureDependency(dependency)

	dependency.ensureDependent(u)
}

// AddDependent adds a dependent to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependent(dependent Component) {
	u.ensureDependent(dependent)

	dependent.ensureDependency(u)
}

// Dependencies returns the dependencies of the Unit.
func (u *Unit) Dependencies() Components {
	u.rLock()
	defer u.rUnlock()

	return u.dependencies
}

// Dependents returns the dependents of the Unit.
func (u *Unit) Dependents() Components {
	u.rLock()
	defer u.rUnlock()

	return u.dependents
}

// ensureDependency adds a dependency to a unit if it's not already present.
func (u *Unit) ensureDependency(dependency Component) {
	u.lock()
	defer u.unlock()

	if !slices.Contains(u.dependencies, dependency) {
		u.dependencies = append(u.dependencies, dependency)
	}
}

// ensureDependent adds a dependent to a unit if it's not already present.
func (u *Unit) ensureDependent(dependent Component) {
	u.lock()
	defer u.unlock()

	if !slices.Contains(u.dependents, dependent) {
		u.dependents = append(u.dependents, dependent)
	}
}

// lock locks the Unit.
func (u *Unit) lock() {
	u.mu.Lock()
}

// unlock unlocks the Unit.
func (u *Unit) unlock() {
	u.mu.Unlock()
}

// rLock locks the Unit for reading.
func (u *Unit) rLock() {
	u.mu.RLock()
}

// rUnlock unlocks the Unit for reading.
func (u *Unit) rUnlock() {
	u.mu.RUnlock()
}

// isProtectedByPreventDestroy returns true if the unit any dependency, or any ancestor dependency, is protected
// by the prevent_destroy flag.
func (u *Unit) isProtectedByPreventDestroy() bool {
	if u.cfg.PreventDestroy != nil && *u.cfg.PreventDestroy {
		return true
	}

	for _, dep := range u.Dependencies() {
		unit, ok := dep.(*Unit)
		if !ok {
			continue
		}

		if unit.isProtectedByPreventDestroy() {
			return true
		}
	}

	return false
}

// isDestroyCommand checks if the current command is a destroy operation
func (u *Unit) isDestroyCommand() bool {
	if u.discoveryContext.Cmd == "destroy" {
		return true
	}

	if u.discoveryContext.Cmd == "apply" && slices.Contains(u.discoveryContext.Args, "-destroy") {
		return true
	}

	return false
}

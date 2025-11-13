package component

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

const (
	UnitKind Kind = "unit"
)

// Unit represents a discovered Terragrunt unit configuration.
// This type serves as a DTO for data exchange between discovery and runner packages.
type Unit struct {
	// Discovery fields (populated by discovery package)
	cfg              *config.TerragruntConfig
	path             string
	reading          []string
	discoveryContext *DiscoveryContext
	dependencies     Components
	dependents       Components
	external         bool

	// Runtime/Execution fields (populated by runner package)
	terragruntOptions    *options.TerragruntOptions
	logger               log.Logger
	assumeAlreadyApplied bool
	flagExcluded         bool

	// Thread-safety
	mu sync.RWMutex
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

// Sources returns the list of sources for this component.
func (u *Unit) Sources() []string {
	if u.cfg == nil || u.cfg.Terraform == nil || u.cfg.Terraform.Source == nil {
		return []string{}
	}

	return []string{*u.cfg.Terraform.Source}
}

// DiscoveryContext returns the discovery context for this component.
func (u *Unit) DiscoveryContext() *DiscoveryContext {
	return u.discoveryContext
}

// SetDiscoveryContext sets the discovery context for this component.
func (u *Unit) SetDiscoveryContext(ctx *DiscoveryContext) {
	u.discoveryContext = ctx
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

// AddDependency adds a dependency to the Unit and vice versa.
//
// Using this method ensure that the dependency graph is properly maintained,
// making it easier to look up dependents and dependencies on a given component
// without the entire graph available.
func (u *Unit) AddDependency(dependency Component) {
	u.ensureDependency(dependency)

	dependency.ensureDependent(u)
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

// TerragruntOptions returns the Terragrunt options for this unit.
func (u *Unit) TerragruntOptions() *options.TerragruntOptions {
	u.rLock()
	defer u.rUnlock()

	return u.terragruntOptions
}

// SetTerragruntOptions sets the Terragrunt options for this unit.
func (u *Unit) SetTerragruntOptions(opts *options.TerragruntOptions) {
	u.lock()
	defer u.unlock()

	u.terragruntOptions = opts
}

// Logger returns the logger for this unit.
func (u *Unit) Logger() log.Logger {
	u.rLock()
	defer u.rUnlock()

	return u.logger
}

// SetLogger sets the logger for this unit.
func (u *Unit) SetLogger(logger log.Logger) {
	u.lock()
	defer u.unlock()

	u.logger = logger
}

// AssumeAlreadyApplied returns whether this unit should be assumed as already applied.
func (u *Unit) AssumeAlreadyApplied() bool {
	u.rLock()
	defer u.rUnlock()

	return u.assumeAlreadyApplied
}

// SetAssumeAlreadyApplied sets whether this unit should be assumed as already applied.
func (u *Unit) SetAssumeAlreadyApplied(assume bool) {
	u.lock()
	defer u.unlock()

	u.assumeAlreadyApplied = assume
}

// FlagExcluded returns whether this unit was excluded by filters/flags.
func (u *Unit) FlagExcluded() bool {
	u.rLock()
	defer u.rUnlock()

	return u.flagExcluded
}

// SetFlagExcluded sets whether this unit was excluded by filters/flags.
func (u *Unit) SetFlagExcluded(excluded bool) {
	u.lock()
	defer u.unlock()

	u.flagExcluded = excluded
}

// String renders this unit as a human-readable string.
func (u *Unit) String() string {
	u.rLock()
	defer u.rUnlock()

	var dependencies = make([]string, 0, len(u.dependencies))
	for _, dependency := range u.dependencies {
		dependencies = append(dependencies, dependency.Path())
	}

	return fmt.Sprintf(
		"Unit %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		u.path, u.flagExcluded, u.assumeAlreadyApplied, strings.Join(dependencies, ", "),
	)
}

// AbsolutePath returns the absolute path of the unit.
// If path conversion fails, returns the original path and logs a warning.
func (u *Unit) AbsolutePath() string {
	u.rLock()
	path := u.path
	logger := u.logger
	u.rUnlock()

	if filepath.IsAbs(path) {
		return path
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		if logger != nil {
			logger.Warnf("Failed to get absolute path for %s: %v", path, err)
		}

		return path
	}

	return absPath
}

// FindUnitInPath returns true if a unit is located under one of the target directories.
// Both unit.Path and targetDirs are expected to be in canonical form (absolute or relative to the same base).
func (u *Unit) FindUnitInPath(targetDirs []string) bool {
	u.rLock()
	defer u.rUnlock()

	return slices.Contains(targetDirs, u.path)
}

// PlanFile returns plan file location, if output folder is set.
func (u *Unit) PlanFile() string {
	u.rLock()
	opts := u.terragruntOptions
	u.rUnlock()

	if opts == nil {
		return ""
	}

	var planFile string

	// set plan file location if output folder is set
	planFile = u.OutputFile()

	planCommand := opts.TerraformCommand == tf.CommandNamePlan || opts.TerraformCommand == tf.CommandNameShow

	// in case if JSON output is enabled, and not specified PlanFile, save plan in working dir
	if planCommand && planFile == "" && opts.JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// OutputFile returns plan file location, if output folder is set.
func (u *Unit) OutputFile() string {
	u.rLock()
	opts := u.terragruntOptions
	logger := u.logger
	u.rUnlock()

	if opts == nil {
		return ""
	}

	return u.getPlanFilePath(logger, opts, opts.OutputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile returns plan JSON file location, if JSON output folder is set.
func (u *Unit) OutputJSONFile() string {
	u.rLock()
	opts := u.terragruntOptions
	logger := u.logger
	u.rUnlock()

	if opts == nil {
		return ""
	}

	return u.getPlanFilePath(logger, opts, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

// getPlanFilePath returns plan file location, if output folder is set.
func (u *Unit) getPlanFilePath(l log.Logger, opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	u.rLock()
	path := u.path
	u.rUnlock()

	relPath, err := filepath.Rel(opts.RootWorkingDir, path)
	if err != nil {
		if l != nil {
			l.Warnf("Failed to get relative path for %s: %v", path, err)
		}
		relPath = path
	}

	dir := filepath.Join(outputFolder, relPath)

	if !filepath.IsAbs(dir) {
		// Resolve relative output folder against root working directory, not the unit working directory,
		// so that artifacts for all units are stored under a single root-level out dir structure.
		base := opts.RootWorkingDir
		if !filepath.IsAbs(base) {
			// In case RootWorkingDir is somehow relative, resolve it first.
			if absBase, err := filepath.Abs(base); err == nil {
				base = absBase
			} else {
				if l != nil {
					l.Warnf("Failed to get absolute path for root working dir %s: %v", base, err)
				}
			}
		}

		dir = filepath.Join(base, dir)

		if absDir, err := filepath.Abs(dir); err == nil {
			dir = absDir
		} else {
			if l != nil {
				l.Warnf("Failed to get absolute path for %s: %v", dir, err)
			}
		}
	}

	return filepath.Join(dir, fileName)
}

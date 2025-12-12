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
	"github.com/gruntwork-io/terragrunt/util"
)

const (
	UnitKind Kind = "unit"
)

// Unit represents a discovered Terragrunt unit configuration.
type Unit struct {
	cfg              *config.TerragruntConfig
	discoveryContext *DiscoveryContext
	Execution        *UnitExecution
	path             string
	reading          []string
	dependencies     Components
	dependents       Components
	mu               sync.RWMutex
	external         bool
	excluded         bool
}

// UnitExecution holds execution-specific fields for running a unit.
// This is nil during discovery phase and populated when preparing for execution.
type UnitExecution struct {
	TerragruntOptions    *options.TerragruntOptions
	Logger               log.Logger
	FlagExcluded         bool
	AssumeAlreadyApplied bool
}

// NewUnit creates a new Unit component with the given path.
func NewUnit(path string) *Unit {
	return &Unit{
		path:             path,
		discoveryContext: &DiscoveryContext{},
		dependencies:     make(Components, 0),
		dependents:       make(Components, 0),
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

// WithDiscoveryContext sets the discovery context for this unit.
func (u *Unit) WithDiscoveryContext(ctx *DiscoveryContext) *Unit {
	u.discoveryContext = ctx

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

// Excluded returns whether the unit was excluded during discovery/filtering.
func (u *Unit) Excluded() bool {
	return u.excluded
}

// SetExcluded marks the unit as excluded during discovery/filtering.
func (u *Unit) SetExcluded(excluded bool) {
	u.excluded = excluded
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

// String renders this unit as a human-readable string for debugging.
//
// Example output:
//
//	Unit /path/to/unit (excluded: false, assume applied: false, dependencies: [/dep1, /dep2])
func (u *Unit) String() string {
	// Snapshot values under read lock to avoid data races
	u.rLock()
	defer u.rUnlock()

	path := u.DisplayPath()
	deps := make([]string, 0, len(u.dependencies))

	for _, dep := range u.dependencies {
		deps = append(deps, dep.DisplayPath())
	}

	excluded := false
	assumeApplied := false

	if u.Execution != nil {
		excluded = u.Execution.FlagExcluded
		assumeApplied = u.Execution.AssumeAlreadyApplied
	}

	return fmt.Sprintf(
		"Unit %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		path, excluded, assumeApplied, strings.Join(deps, ", "),
	)
}

// AbsolutePath returns the absolute path of the unit.
// If path conversion fails, returns the original path and logs a warning if a logger is available.
func (u *Unit) AbsolutePath() string {
	absPath, err := filepath.Abs(u.path)
	if err != nil {
		if u.Execution != nil && u.Execution.Logger != nil {
			u.Execution.Logger.Warnf("Failed to convert unit path %q to absolute path: %v", u.path, err)
		}

		return u.path
	}

	return absPath
}

// DisplayPath returns the path relative to DiscoveryContext.WorkingDir for display purposes.
// Falls back to the original path if relative path calculation fails or WorkingDir is empty.
func (u *Unit) DisplayPath() string {
	if u.discoveryContext == nil || u.discoveryContext.WorkingDir == "" {
		return u.path
	}

	if rel, err := filepath.Rel(u.discoveryContext.WorkingDir, u.path); err == nil {
		return rel
	}

	return u.path
}

// FindInPaths returns true if the unit is located in one of the target directories.
// Paths are normalized before comparison to handle absolute/relative path mismatches.
func (u *Unit) FindInPaths(targetDirs []string) bool {
	cleanUnitPath := util.CleanPath(u.path)

	for _, dir := range targetDirs {
		cleanDir := util.CleanPath(dir)
		if util.HasPathPrefix(cleanUnitPath, cleanDir) {
			return true
		}
	}

	return false
}

// PlanFile returns plan file location if output folder is set.
// Requires Execution to be populated.
func (u *Unit) PlanFile(opts *options.TerragruntOptions) string {
	if u.Execution == nil || u.Execution.TerragruntOptions == nil {
		return ""
	}

	planFile := u.OutputFile(opts)

	planCommand := u.Execution.TerragruntOptions.TerraformCommand == tf.CommandNamePlan ||
		u.Execution.TerragruntOptions.TerraformCommand == tf.CommandNameShow

	// if JSON output enabled and no PlanFile specified, save plan in working dir
	if planCommand && planFile == "" && u.Execution.TerragruntOptions.JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// OutputFile returns plan file location if output folder is set.
func (u *Unit) OutputFile(opts *options.TerragruntOptions) string {
	return u.planFilePath(opts, opts.OutputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile returns plan JSON file location if JSON output folder is set.
func (u *Unit) OutputJSONFile(opts *options.TerragruntOptions) string {
	return u.planFilePath(opts, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

// planFilePath computes the path for plan output files.
func (u *Unit) planFilePath(opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	relPath, err := filepath.Rel(opts.RootWorkingDir, u.path)
	if err != nil {
		relPath = u.path
	}

	dir := filepath.Join(outputFolder, relPath)

	if !filepath.IsAbs(dir) {
		base := opts.RootWorkingDir
		if !filepath.IsAbs(base) {
			if absBase, err := filepath.Abs(base); err == nil {
				base = absBase
			}
		}

		dir = filepath.Join(base, dir)

		if absDir, err := filepath.Abs(dir); err == nil {
			dir = absDir
		}
	}

	return filepath.Join(dir, fileName)
}

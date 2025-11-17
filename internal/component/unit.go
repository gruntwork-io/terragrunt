package component

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/terragrunt/config"
	runnertypes "github.com/gruntwork-io/terragrunt/internal/runner/types"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"

	xsync "github.com/puzpuzpuz/xsync/v3"
)

const (
	UnitKind Kind = "unit"
)

// Units is a collection of Unit pointers.
type Units []*Unit

// UnitsMap is a map of paths to Unit pointers.
type UnitsMap map[string]*Unit

// per-path output locks to serialize flushes for the same unit
var (
	unitOutputLocks = xsync.NewMapOf[string, *sync.Mutex]()
)

func getUnitOutputLock(path string) *sync.Mutex {
	if mu, ok := unitOutputLocks.Load(path); ok {
		return mu
	}
	// Create a new mutex and attempt to store it; if another goroutine stored one first,
	// use the existing mutex returned by LoadOrStore.
	newMu := &sync.Mutex{}
	actual, _ := unitOutputLocks.LoadOrStore(path, newMu)

	return actual
}

// Unit represents a Terragrunt unit configuration.
// This type is used by both discovery and runner packages.
// It contains all fields needed throughout the unit's lifecycle.
type Unit struct {
	logger               log.Logger
	cfg                  *config.TerragruntConfig
	discoveryContext     *DiscoveryContext
	executionOptions     *runnertypes.RunnerOptions
	path                 string
	reading              []string
	dependencies         Components
	dependents           Components
	mu                   sync.RWMutex
	external             bool
	assumeAlreadyApplied bool
	flagExcluded         bool
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

// ExecutionOptions returns the execution options for this unit.
func (u *Unit) ExecutionOptions() *runnertypes.RunnerOptions {
	u.rLock()
	defer u.rUnlock()

	return u.executionOptions
}

// SetExecutionOptions sets the execution options for this unit.
func (u *Unit) SetExecutionOptions(opts *runnertypes.RunnerOptions) {
	u.lock()
	defer u.unlock()

	u.executionOptions = opts
}

// SetTerragruntOptions sets the execution options from TerragruntOptions.
// This extracts only the 11 fields needed by component.Unit from the full TerragruntOptions.
func (u *Unit) SetTerragruntOptions(opts *options.TerragruntOptions) {
	if opts == nil {
		u.SetExecutionOptions(nil)
		return
	}

	executionOptions := &runnertypes.RunnerOptions{
		Writer:                      opts.Writer,
		ErrWriter:                   opts.ErrWriter,
		TerraformCommand:            opts.TerraformCommand,
		OutputFolder:                opts.OutputFolder,
		JSONOutputFolder:            opts.JSONOutputFolder,
		RootWorkingDir:              opts.RootWorkingDir,
		TerraformCliArgs:            opts.TerraformCliArgs,
		WorkingDir:                  opts.WorkingDir,
		TerragruntConfigPath:        opts.TerragruntConfigPath,
		IncludeExternalDependencies: opts.IncludeExternalDependencies,
		NonInteractive:              opts.NonInteractive,
	}

	u.SetExecutionOptions(executionOptions)
}

// TerragruntOptions returns a minimal TerragruntOptions for backward compatibility.
// This is used by legacy code that expects TerragruntOptions.
// DEPRECATED: Use ExecutionOptions() instead to access individual fields directly.
func (u *Unit) TerragruntOptions() *options.TerragruntOptions {
	u.rLock()
	opts := u.executionOptions
	u.rUnlock()

	if opts == nil {
		return nil
	}

	return &options.TerragruntOptions{
		Writer:                      opts.Writer,
		ErrWriter:                   opts.ErrWriter,
		TerraformCommand:            opts.TerraformCommand,
		OutputFolder:                opts.OutputFolder,
		JSONOutputFolder:            opts.JSONOutputFolder,
		RootWorkingDir:              opts.RootWorkingDir,
		TerraformCliArgs:            opts.TerraformCliArgs,
		WorkingDir:                  opts.WorkingDir,
		TerragruntConfigPath:        opts.TerragruntConfigPath,
		IncludeExternalDependencies: opts.IncludeExternalDependencies,
		NonInteractive:              opts.NonInteractive,
	}
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

// FlushOutput flushes buffer data to the output writer.
// This method is used by the runner when ExecutionOptions.Writer is a UnitWriter.
func (u *Unit) FlushOutput() error {
	u.rLock()
	opts := u.executionOptions
	u.rUnlock()

	if opts == nil || opts.Writer == nil {
		return nil
	}

	if writer, ok := opts.Writer.(*UnitWriter); ok {
		key := u.AbsolutePath()

		mu := getUnitOutputLock(key)

		mu.Lock()
		defer mu.Unlock()

		return writer.Flush()
	}

	return nil
}

// PlanFile returns plan file location, if output folder is set.
// This version uses the unit's stored ExecutionOptions.
func (u *Unit) PlanFile() string {
	u.rLock()
	opts := u.executionOptions
	logger := u.logger
	u.rUnlock()

	if opts == nil {
		return ""
	}

	return u.planFileWithExecutionOptions(logger, opts)
}

// planFileWithExecutionOptions returns plan file location with execution options.
func (u *Unit) planFileWithExecutionOptions(l log.Logger, opts *runnertypes.RunnerOptions) string {
	if opts == nil {
		return ""
	}

	// set plan file location if output folder is set
	planFile := u.outputFileWithExecutionOptions(l, opts)

	planCommand := opts.TerraformCommand == tf.CommandNamePlan || opts.TerraformCommand == tf.CommandNameShow

	// in case if JSON output is enabled, and not specified PlanFile, save plan in working dir
	if planCommand && planFile == "" && opts.JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// GetOutputFile returns plan file location, if output folder is set.
// This version uses the unit's stored ExecutionOptions.
func (u *Unit) GetOutputFile() string {
	u.rLock()
	opts := u.executionOptions
	logger := u.logger
	u.rUnlock()

	if opts == nil || opts.OutputFolder == "" {
		return ""
	}

	return u.outputFileWithExecutionOptions(logger, opts)
}

// outputFileWithExecutionOptions returns plan file location with execution options.
func (u *Unit) outputFileWithExecutionOptions(l log.Logger, opts *runnertypes.RunnerOptions) string {
	return u.getPlanFilePath(l, opts.RootWorkingDir, opts.OutputFolder, tf.TerraformPlanFile)
}

// GetOutputJSONFile returns plan JSON file location, if JSON output folder is set.
// This version uses the unit's stored ExecutionOptions.
func (u *Unit) GetOutputJSONFile() string {
	u.rLock()
	opts := u.executionOptions
	logger := u.logger
	u.rUnlock()

	if opts == nil || opts.JSONOutputFolder == "" {
		return ""
	}

	return u.outputJSONFileWithExecutionOptions(logger, opts)
}

// outputJSONFileWithExecutionOptions returns plan JSON file location with execution options.
func (u *Unit) outputJSONFileWithExecutionOptions(l log.Logger, opts *runnertypes.RunnerOptions) string {
	return u.getPlanFilePath(l, opts.RootWorkingDir, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

// getPlanFilePath returns plan file location with explicit parameters.
func (u *Unit) getPlanFilePath(l log.Logger, rootWorkingDir, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	u.rLock()
	path := u.path
	u.rUnlock()

	relPath, err := filepath.Rel(rootWorkingDir, path)
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
		base := rootWorkingDir
		if !filepath.IsAbs(base) {
			// In case RootWorkingDir is somehow relative, resolve it first.
			if absBase, err := filepath.Abs(base); err == nil {
				base = absBase
			} else if l != nil {
				l.Warnf("Failed to get absolute path for root working dir %s: %v", base, err)
			}
		}

		dir = filepath.Join(base, dir)

		if absDir, err := filepath.Abs(dir); err == nil {
			dir = absDir
		} else if l != nil {
			l.Warnf("Failed to get absolute path for %s: %v", dir, err)
		}
	}

	return filepath.Join(dir, fileName)
}

// SortedKeys returns the keys for the given map in sorted order.
// This is used to ensure we always iterate over maps of units in a consistent order.
func (unitsMap UnitsMap) SortedKeys() []string {
	keys := make([]string, 0, len(unitsMap))
	for key := range unitsMap {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// FindByPath returns the unit that matches the given path, or nil if no such unit exists in the map.
func (unitsMap UnitsMap) FindByPath(path string) *Unit {
	if unit, ok := unitsMap[path]; ok {
		return unit
	}

	return nil
}

// MergeMaps merges the given external dependencies into the given map of units if those dependencies
// aren't already in the units map.
func (unitsMap UnitsMap) MergeMaps(externalDependencies UnitsMap) UnitsMap {
	out := UnitsMap{}

	maps.Copy(out, externalDependencies)
	maps.Copy(out, unitsMap)

	return out
}

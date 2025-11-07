package component

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/puzpuzpuz/xsync/v3"
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

// Content migrated directly from the internal/runner/common package.
//
// It wasn't moved over with a lot of thought. It's possible this can be organized a bit better later.

type Units []*Unit

type UnitsMap map[string]*Unit

// String renders this unit as a human-readable string
func (u *Unit) String() string {
	var dependencies = make([]string, 0, len(u.Dependencies()))
	for _, dependency := range u.Dependencies() {
		dependencies = append(dependencies, dependency.Path())
	}

	assumeApplied := u.External() && !u.ShouldApplyExternal()

	return fmt.Sprintf(
		"Unit %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		u.Path(), u.Excluded(), assumeApplied, strings.Join(dependencies, ", "),
	)
}

// FlushOutput flushes buffer data to the output writer.
func (u *Unit) FlushOutput(l log.Logger) error {
	if u == nil || u.Opts() == nil || u.Opts().Writer == nil {
		return nil
	}

	if writer, ok := u.Opts().Writer.(*UnitWriter); ok {
		key := u.AbsolutePath(l)

		mu := getUnitOutputLock(key)

		mu.Lock()
		defer mu.Unlock()

		return writer.Flush()
	}

	return nil
}

// PlanFile - return plan file location, if output folder is set
func (u *Unit) PlanFile(l log.Logger, opts *options.TerragruntOptions) string {
	var planFile string

	// set plan file location if output folder is set
	planFile = u.OutputFile(l, opts)

	planCommand := u.Opts().TerraformCommand == tf.CommandNamePlan || u.Opts().TerraformCommand == tf.CommandNameShow

	// in case if JSON output is enabled, and not specified PlanFile, save plan in working dir
	if planCommand && planFile == "" && u.Opts().JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// OutputFile - return plan file location, if output folder is set
func (u *Unit) OutputFile(l log.Logger, opts *options.TerragruntOptions) string {
	return u.getPlanFilePath(l, opts, opts.OutputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile - return plan JSON file location, if JSON output folder is set
func (u *Unit) OutputJSONFile(l log.Logger, opts *options.TerragruntOptions) string {
	return u.getPlanFilePath(l, opts, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

// getPlanFilePath - return plan graph file location, if output folder is set
func (u *Unit) getPlanFilePath(l log.Logger, opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	path, err := filepath.Rel(opts.RootWorkingDir, u.Path())
	if err != nil {
		l.Warnf("Failed to get relative path for %s: %v", u.Path(), err)
		path = u.Path()
	}

	dir := filepath.Join(outputFolder, path)

	if !filepath.IsAbs(dir) {
		// Resolve relative output folder against root working directory, not the unit working directory,
		// so that artifacts for all units are stored under a single root-level out dir structure.
		base := opts.RootWorkingDir
		if !filepath.IsAbs(base) {
			// In case RootWorkingDir is somehow relative, resolve it first.
			if absBase, err := filepath.Abs(base); err == nil {
				base = absBase
			} else {
				l.Warnf("Failed to get absolute path for root working dir %s: %v", base, err)
			}
		}

		dir = filepath.Join(base, dir)

		if absDir, err := filepath.Abs(dir); err == nil {
			dir = absDir
		} else {
			l.Warnf("Failed to get absolute path for %s: %v", dir, err)
		}
	}

	return filepath.Join(dir, fileName)
}

// FindUnitInPath returns true if a unit is located under one of the target directories.
// Both unit.Path and targetDirs are expected to be in canonical form (absolute or relative to the same base).
func (u *Unit) FindUnitInPath(targetDirs []string) bool {
	return slices.Contains(targetDirs, u.Path())
}

// AbsolutePath returns the absolute path of the unit.
// If path conversion fails, returns the original path and logs a warning.
func (u *Unit) AbsolutePath(l log.Logger) string {
	if filepath.IsAbs(u.Path()) {
		return u.Path()
	}

	absPath, err := filepath.Abs(u.Path())
	if err != nil {
		if l != nil {
			l.Warnf("Failed to get absolute path for %s: %v", u.Path(), err)
		}

		return u.Path()
	}

	return absPath
}

// getDependenciesForUnit Get the list of units this unit depends on
func (u *Unit) getDependenciesForUnit(unitsMap UnitsMap, terragruntConfigPaths []string) (Units, error) {
	dependencies := Units{}

	if u.Config() == nil ||
		u.Config().Dependencies == nil ||
		len(u.Config().Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range u.Config().Dependencies.Paths {
		dependencyUnitPath, err := util.CanonicalPath(dependencyPath, u.Path())
		if err != nil {
			return dependencies, errors.Errorf("failed to resolve canonical path for dependency %s: %w", dependencyPath, err)
		}

		if files.FileExists(dependencyUnitPath) && !files.IsDir(dependencyUnitPath) {
			dependencyUnitPath = filepath.Dir(dependencyUnitPath)
		}

		dependencyUnit, foundUnit := unitsMap[dependencyUnitPath]
		if !foundUnit {
			dependencyErr := UnrecognizedDependencyError{
				UnitPath:              u.Path(),
				DependencyPath:        dependencyPath,
				TerragruntConfigPaths: terragruntConfigPaths,
			}

			return dependencies, dependencyErr
		}

		dependencies = append(dependencies, dependencyUnit)
	}

	return dependencies, nil
}

// MergeMaps the given external dependencies into the given map of units if those dependencies aren't already in the
// units map
func (unitsMap UnitsMap) MergeMaps(externalDependencies UnitsMap) UnitsMap {
	out := UnitsMap{}

	maps.Copy(out, externalDependencies)

	maps.Copy(out, unitsMap)

	return out
}

// FindByPath returns the unit that matches the given path, or nil if no such unit exists in the map.
func (unitsMap UnitsMap) FindByPath(path string) *Unit {
	if unit, ok := unitsMap[path]; ok {
		return unit
	}

	return nil
}

// ConvertDiscoveryToRunner converts units from discovery domain to runner domain by resolving
// Component interface dependencies into concrete *Unit pointer dependencies.
// Discovery found all dependencies and stored them as Component interfaces, but runner needs
// concrete *Unit pointers for efficient execution. This function translates between domains.
func (unitsMap UnitsMap) ConvertDiscoveryToRunner(canonicalTerragruntConfigPaths []string) (Units, error) {
	units := Units{}

	keys := unitsMap.SortedKeys()

	for _, key := range keys {
		unit := unitsMap[key]

		dependencies, err := unit.getDependenciesForUnit(unitsMap, canonicalTerragruntConfigPaths)
		if err != nil {
			return units, err
		}

		for _, dep := range dependencies {
			unit.AddDependency(dep)
		}

		units = append(units, unit)
	}

	return units, nil
}

// SortedKeys Return the keys for the given map in sorted order. This is used to ensure we always iterate over maps of units
// in a consistent order (Go does not guarantee iteration order for maps, and usually makes it random)
func (unitsMap UnitsMap) SortedKeys() []string {
	keys := make([]string, 0, len(unitsMap))
	for key := range unitsMap {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

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

// EnsureAbsolutePath ensures a path is absolute, converting it if necessary.
// Returns the absolute path and any error encountered during conversion.
func EnsureAbsolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Errorf("failed to get absolute path for %s: %w", path, err)
	}

	return absPath, nil
}

// UnitWriter represents a Writer with data buffering.
// We should avoid outputting data directly to the output out,
// since when units run in parallel, the output data may be mixed with each other, thereby spoiling each other's results.
type UnitWriter struct {
	buffer *bytes.Buffer
	out    io.Writer
	mu     sync.Mutex
}

// NewUnitWriter returns a new UnitWriter instance.
func NewUnitWriter(out io.Writer) *UnitWriter {
	return &UnitWriter{
		buffer: &bytes.Buffer{},
		out:    out,
	}
}

// Write appends the contents of p to the buffer.
func (writer *UnitWriter) Write(p []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	n, err := writer.buffer.Write(p)
	if err != nil {
		return n, errors.New(err)
	}

	// If the last byte is a newline character, flush the buffer early.
	if writer.buffer.Len() > 0 {
		if p[len(p)-1] == '\n' {
			if err := writer.flushUnsafe(); err != nil {
				return n, errors.New(err)
			}
		}
	}

	return n, nil
}

// Flush flushes buffer data to the `out` writer.
func (writer *UnitWriter) Flush() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	return writer.flushUnsafe()
}

// flushUnsafe flushes buffer data to the `out` writer.
// Must be called with writer.mu held.
func (writer *UnitWriter) flushUnsafe() error {
	if _, err := fmt.Fprint(writer.out, writer.buffer); err != nil {
		return errors.New(err)
	}

	writer.buffer.Reset()

	return nil
}

type UnrecognizedDependencyError struct {
	UnitPath              string
	DependencyPath        string
	TerragruntConfigPaths []string
}

func (err UnrecognizedDependencyError) Error() string {
	return errors.Errorf(
		"Unit %s specifies %s as a dependency, but that dependency was not one of the ones found while scanning subfolders: %v",
		err.UnitPath,
		err.DependencyPath,
		err.TerragruntConfigPaths,
	).Error()
}

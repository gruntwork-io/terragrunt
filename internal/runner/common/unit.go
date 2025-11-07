package common

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"

	xsync "github.com/puzpuzpuz/xsync/v3"
)

// Unit represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type Unit struct {
	TerragruntOptions *options.TerragruntOptions
	Component         *component.Unit
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

type Units []*Unit

type UnitsMap map[string]*Unit

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

// String renders this unit as a human-readable string
func (unit *Unit) String() string {
	var dependencies = make([]string, 0, len(unit.Component.Dependencies()))
	for _, dependency := range unit.Component.Dependencies() {
		dependencies = append(dependencies, dependency.Path())
	}

	assumeApplied := unit.Component.External() && !unit.Component.ShouldApplyExternal()

	return fmt.Sprintf(
		"Unit %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		unit.Component.Path(), unit.Component.Excluded(), assumeApplied, strings.Join(dependencies, ", "),
	)
}

// FlushOutput flushes buffer data to the output writer.
func (unit *Unit) FlushOutput(l log.Logger) error {
	if unit == nil || unit.TerragruntOptions == nil || unit.TerragruntOptions.Writer == nil {
		return nil
	}

	if writer, ok := unit.TerragruntOptions.Writer.(*UnitWriter); ok {
		key := unit.AbsolutePath(l)

		mu := getUnitOutputLock(key)

		mu.Lock()
		defer mu.Unlock()

		return writer.Flush()
	}

	return nil
}

// PlanFile - return plan file location, if output folder is set
func (unit *Unit) PlanFile(l log.Logger, opts *options.TerragruntOptions) string {
	var planFile string

	// set plan file location if output folder is set
	planFile = unit.OutputFile(l, opts)

	planCommand := unit.TerragruntOptions.TerraformCommand == tf.CommandNamePlan || unit.TerragruntOptions.TerraformCommand == tf.CommandNameShow

	// in case if JSON output is enabled, and not specified PlanFile, save plan in working dir
	if planCommand && planFile == "" && unit.TerragruntOptions.JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// OutputFile - return plan file location, if output folder is set
func (unit *Unit) OutputFile(l log.Logger, opts *options.TerragruntOptions) string {
	return unit.getPlanFilePath(l, opts, opts.OutputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile - return plan JSON file location, if JSON output folder is set
func (unit *Unit) OutputJSONFile(l log.Logger, opts *options.TerragruntOptions) string {
	return unit.getPlanFilePath(l, opts, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

// getPlanFilePath - return plan graph file location, if output folder is set
func (unit *Unit) getPlanFilePath(l log.Logger, opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	path, err := filepath.Rel(opts.RootWorkingDir, unit.Component.Path())
	if err != nil {
		l.Warnf("Failed to get relative path for %s: %v", unit.Component.Path(), err)
		path = unit.Component.Path()
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
func (unit *Unit) FindUnitInPath(targetDirs []string) bool {
	return slices.Contains(targetDirs, unit.Component.Path())
}

// AbsolutePath returns the absolute path of the unit.
// If path conversion fails, returns the original path and logs a warning.
func (unit *Unit) AbsolutePath(l log.Logger) string {
	if filepath.IsAbs(unit.Component.Path()) {
		return unit.Component.Path()
	}

	absPath, err := filepath.Abs(unit.Component.Path())
	if err != nil {
		if l != nil {
			l.Warnf("Failed to get absolute path for %s: %v", unit.Component.Path(), err)
		}

		return unit.Component.Path()
	}

	return absPath
}

// getDependenciesForUnit Get the list of units this unit depends on
func (unit *Unit) getDependenciesForUnit(unitsMap UnitsMap, terragruntConfigPaths []string) (Units, error) {
	dependencies := Units{}

	if unit.Component == nil ||
		unit.Component.Config() == nil ||
		unit.Component.Config().Dependencies == nil ||
		len(unit.Component.Config().Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range unit.Component.Config().Dependencies.Paths {
		dependencyUnitPath, err := util.CanonicalPath(dependencyPath, unit.Component.Path())
		if err != nil {
			return dependencies, errors.Errorf("failed to resolve canonical path for dependency %s: %w", dependencyPath, err)
		}

		if files.FileExists(dependencyUnitPath) && !files.IsDir(dependencyUnitPath) {
			dependencyUnitPath = filepath.Dir(dependencyUnitPath)
		}

		dependencyUnit, foundUnit := unitsMap[dependencyUnitPath]
		if !foundUnit {
			dependencyErr := UnrecognizedDependencyError{
				UnitPath:              unit.Component.Path(),
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
			unit.Component.AddDependency(dep.Component)
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

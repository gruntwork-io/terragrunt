package runbase

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

// Unit represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type Unit struct {
	TerragruntOptions    *options.TerragruntOptions
	Logger               log.Logger
	Path                 string
	Dependencies         Units
	Config               config.TerragruntConfig
	AssumeAlreadyApplied bool
	FlagExcluded         bool
}

type Units []*Unit

type UnitsMap map[string]*Unit

// String renders this module as a human-readable string
func (unit *Unit) String() string {
	dependencies := []string{}
	for _, dependency := range unit.Dependencies {
		dependencies = append(dependencies, dependency.Path)
	}

	return fmt.Sprintf(
		"Unit %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		unit.Path, unit.FlagExcluded, unit.AssumeAlreadyApplied, strings.Join(dependencies, ", "),
	)
}

// FlushOutput flushes buffer data to the output writer.
func (unit *Unit) FlushOutput() error {
	if writer, ok := unit.TerragruntOptions.Writer.(*UnitWriter); ok {
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

func (unit *Unit) getPlanFilePath(l log.Logger, opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	path, _ := filepath.Rel(opts.WorkingDir, unit.Path)
	dir := filepath.Join(outputFolder, path)

	if !filepath.IsAbs(dir) {
		dir = filepath.Join(opts.WorkingDir, dir)
		if absDir, err := filepath.Abs(dir); err == nil {
			dir = absDir
		} else {
			l.Warnf("Failed to get absolute path for %s: %v", dir, err)
		}
	}

	return filepath.Join(dir, fileName)
}

// FindUnitInPath returns true if a unit is located under one of the target directories
func (unit *Unit) FindUnitInPath(targetDirs []string) bool {
	return slices.Contains(targetDirs, unit.Path)
}

// Get the list of units this unit depends on
func (unit *Unit) getDependenciesForUnit(unitsMap UnitsMap, terragruntConfigPaths []string) (Units, error) {
	dependencies := Units{}

	if unit.Config.Dependencies == nil || len(unit.Config.Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range unit.Config.Dependencies.Paths {
		dependencyUnitPath, err := util.CanonicalPath(dependencyPath, unit.Path)
		if err != nil {
			// TODO: Remove lint suppression
			return dependencies, nil //nolint:nilerr
		}

		if files.FileExists(dependencyUnitPath) && !files.IsDir(dependencyUnitPath) {
			dependencyUnitPath = filepath.Dir(dependencyUnitPath)
		}

		dependencyUnit, foundUnit := unitsMap[dependencyUnitPath]
		if !foundUnit {
			err := UnrecognizedDependencyError{
				UnitPath:              unit.Path,
				DependencyPath:        dependencyPath,
				TerragruntConfigPaths: terragruntConfigPaths,
			}

			return dependencies, errors.New(err)
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

func (unitsMap UnitsMap) FindByPath(path string) *Unit {
	for _, unit := range unitsMap {
		if unit.Path == path {
			return unit
		}
	}

	return nil
}

// CrossLinkDependencies Go through each unit in the given map and cross-link its dependencies to the other units in that same map. If
// a dependency is referenced that is not in the given map, return an error.
func (unitsMap UnitsMap) CrossLinkDependencies(canonicalTerragruntConfigPaths []string) (Units, error) {
	units := Units{}

	keys := unitsMap.SortedKeys()

	for _, key := range keys {
		unit := unitsMap[key]

		dependencies, err := unit.getDependenciesForUnit(unitsMap, canonicalTerragruntConfigPaths)
		if err != nil {
			return units, err
		}

		unit.Dependencies = dependencies
		units = append(units, unit)
	}

	return units, nil
}

// WriteDot is used to emit a GraphViz compatible definition
// for a directed graph. It can be used to dump a .dot file.
// This is a similar implementation to terraform's digraph https://github.com/hashicorp/terraform/blob/master/digraph/graphviz.go
// adding some styling to units that are excluded from the execution in *-all commands
func (units Units) WriteDot(l log.Logger, w io.Writer, opts *options.TerragruntOptions) error {
	if _, err := w.Write([]byte("digraph {\n")); err != nil {
		return errors.New(err)
	}
	defer func(w io.Writer, p []byte) {
		_, err := w.Write(p)
		if err != nil {
			l.Warnf("Failed to close graphviz output: %v", err)
		}
	}(w, []byte("}\n"))

	// all paths are relative to the TerragruntConfigPath
	prefix := filepath.Dir(opts.TerragruntConfigPath) + "/"

	for _, source := range units {
		// apply a different coloring for excluded nodes
		style := ""
		if source.FlagExcluded {
			style = "[color=red]"
		}

		nodeLine := fmt.Sprintf("\t\"%s\" %s;\n",
			strings.TrimPrefix(source.Path, prefix), style)

		_, err := w.Write([]byte(nodeLine))
		if err != nil {
			return errors.New(err)
		}

		for _, target := range source.Dependencies {
			line := fmt.Sprintf("\t\"%s\" -> \"%s\";\n",
				strings.TrimPrefix(source.Path, prefix),
				strings.TrimPrefix(target.Path, prefix),
			)

			_, err := w.Write([]byte(line))
			if err != nil {
				return errors.New(err)
			}
		}
	}

	return nil
}

// CheckForCycles checks for dependency cycles in the given list of units and return an error if one is found.
func (units Units) CheckForCycles() error {
	visitedPaths := []string{}
	currentTraversalPaths := []string{}

	for _, unit := range units {
		err := checkForCyclesUsingDepthFirstSearch(unit, &visitedPaths, &currentTraversalPaths)
		if err != nil {
			return err
		}
	}

	return nil
}

// Return the keys for the given map in sorted order. This is used to ensure we always iterate over maps of units
// in a consistent order (Go does not guarantee iteration order for maps, and usually makes it random)
func (unitsMap UnitsMap) SortedKeys() []string {
	keys := []string{}
	for key := range unitsMap {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// Check for cycles using a depth-first-search as described here:
// https://en.wikipedia.org/wiki/Topological_sorting#Depth-first_search
//
// Note that this method uses two lists, visitedPaths, and currentTraversalPaths, to track what nodes have already been
// seen. We need to use lists to maintain ordering so we can show the proper order of paths in a cycle. Of course, a
// list doesn't perform well with repeated contains() and remove() checks, so ideally we'd use an ordered Map (e.g.
// Java's LinkedHashMap), but since Go doesn't have such a data structure built-in, and our lists are going to be very
// small (at most, a few dozen paths), there is no point in worrying about performance.
func checkForCyclesUsingDepthFirstSearch(unit *Unit, visitedPaths *[]string, currentTraversalPaths *[]string) error {
	if util.ListContainsElement(*visitedPaths, unit.Path) {
		return nil
	}

	if util.ListContainsElement(*currentTraversalPaths, unit.Path) {
		return errors.New(DependencyCycleError(append(*currentTraversalPaths, unit.Path)))
	}

	*currentTraversalPaths = append(*currentTraversalPaths, unit.Path)
	for _, dependency := range unit.Dependencies {
		if err := checkForCyclesUsingDepthFirstSearch(dependency, visitedPaths, currentTraversalPaths); err != nil {
			return err
		}
	}

	*visitedPaths = append(*visitedPaths, unit.Path)
	*currentTraversalPaths = util.RemoveElementFromList(*currentTraversalPaths, unit.Path)

	return nil
}

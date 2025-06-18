package common

import (
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/report"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf"
)

// Stack represents a stack of units that you can "spin up" or "spin down"
type Stack struct {
	Report                *report.Report
	TerragruntOptions     *options.TerragruntOptions
	ChildTerragruntConfig *config.TerragruntConfig
	Units                 Units
	ParserOptions         []hclparse.Option
}

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
func (module *Unit) String() string {
	dependencies := []string{}
	for _, dependency := range module.Dependencies {
		dependencies = append(dependencies, dependency.Path)
	}

	return fmt.Sprintf(
		"Module %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		module.Path, module.FlagExcluded, module.AssumeAlreadyApplied, strings.Join(dependencies, ", "),
	)
}

// FlushOutput flushes buffer data to the output writer.
func (module *Unit) FlushOutput(runner StackRunner) error {
	if writer, ok := module.TerragruntOptions.Writer.(*ModuleWriter); ok {
		runner.Lock()
		defer runner.Unlock()

		return writer.Flush()
	}

	return nil
}

// PlanFile - return plan file location, if output folder is set
func (module *Unit) PlanFile(l log.Logger, opts *options.TerragruntOptions) string {
	var planFile string

	// set plan file location if output folder is set
	planFile = module.OutputFile(l, opts)

	planCommand := module.TerragruntOptions.TerraformCommand == tf.CommandNamePlan || module.TerragruntOptions.TerraformCommand == tf.CommandNameShow

	// in case if JSON output is enabled, and not specified PlanFile, save plan in working dir
	if planCommand && planFile == "" && module.TerragruntOptions.JSONOutputFolder != "" {
		planFile = tf.TerraformPlanFile
	}

	return planFile
}

// OutputFile - return plan file location, if output folder is set
func (module *Unit) OutputFile(l log.Logger, opts *options.TerragruntOptions) string {
	return module.getPlanFilePath(l, opts, opts.OutputFolder, tf.TerraformPlanFile)
}

// OutputJSONFile - return plan JSON file location, if JSON output folder is set
func (module *Unit) OutputJSONFile(l log.Logger, opts *options.TerragruntOptions) string {
	return module.getPlanFilePath(l, opts, opts.JSONOutputFolder, tf.TerraformPlanJSONFile)
}

func (module *Unit) getPlanFilePath(l log.Logger, opts *options.TerragruntOptions, outputFolder, fileName string) string {
	if outputFolder == "" {
		return ""
	}

	path, _ := filepath.Rel(opts.WorkingDir, module.Path)
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

// FindModuleInPath returns true if a module is located under one of the target directories
func (module *Unit) FindModuleInPath(targetDirs []string) bool {
	return slices.Contains(targetDirs, module.Path)
}

// Get the list of modules this module depends on
func (module *Unit) getDependenciesForModule(modulesMap UnitsMap, terragruntConfigPaths []string) (Units, error) {
	dependencies := Units{}

	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range module.Config.Dependencies.Paths {
		dependencyModulePath, err := util.CanonicalPath(dependencyPath, module.Path)
		if err != nil {
			// TODO: Remove lint suppression
			return dependencies, nil //nolint:nilerr
		}

		if files.FileExists(dependencyModulePath) && !files.IsDir(dependencyModulePath) {
			dependencyModulePath = filepath.Dir(dependencyModulePath)
		}

		dependencyModule, foundModule := modulesMap[dependencyModulePath]
		if !foundModule {
			err := UnrecognizedDependencyError{
				ModulePath:            module.Path,
				DependencyPath:        dependencyPath,
				TerragruntConfigPaths: terragruntConfigPaths,
			}

			return dependencies, errors.New(err)
		}

		dependencies = append(dependencies, dependencyModule)
	}

	return dependencies, nil
}

// Merge the given external dependencies into the given map of modules if those dependencies aren't already in the
// modules map
func (unitsMap UnitsMap) MergeMaps(externalDependencies UnitsMap) UnitsMap {
	out := UnitsMap{}

	maps.Copy(out, externalDependencies)

	maps.Copy(out, unitsMap)

	return out
}

func (unitsMap UnitsMap) FindByPath(path string) *Unit {
	for _, module := range unitsMap {
		if module.Path == path {
			return module
		}
	}

	return nil
}

// Go through each module in the given map and cross-link its dependencies to the other modules in that same map. If
// a dependency is referenced that is not in the given map, return an error.
func (modulesMap UnitsMap) CrossLinkDependencies(canonicalTerragruntConfigPaths []string) (Units, error) {
	modules := Units{}

	keys := modulesMap.SortedKeys()

	for _, key := range keys {
		module := modulesMap[key]

		dependencies, err := module.getDependenciesForModule(modulesMap, canonicalTerragruntConfigPaths)
		if err != nil {
			return modules, err
		}

		module.Dependencies = dependencies
		modules = append(modules, module)
	}

	return modules, nil
}

// WriteDot is used to emit a GraphViz compatible definition
// for a directed graph. It can be used to dump a .dot file.
// This is a similar implementation to terraform's digraph https://github.com/hashicorp/terraform/blob/master/digraph/graphviz.go
// adding some styling to modules that are excluded from the execution in *-all commands
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

// CheckForCycles checks for dependency cycles in the given list of modules and return an error if one is found.
func (units Units) CheckForCycles() error {
	visitedPaths := []string{}
	currentTraversalPaths := []string{}

	for _, module := range units {
		err := checkForCyclesUsingDepthFirstSearch(module, &visitedPaths, &currentTraversalPaths)
		if err != nil {
			return err
		}
	}

	return nil
}

// Return the keys for the given map in sorted order. This is used to ensure we always iterate over maps of modules
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
func checkForCyclesUsingDepthFirstSearch(module *Unit, visitedPaths *[]string, currentTraversalPaths *[]string) error {
	if util.ListContainsElement(*visitedPaths, module.Path) {
		return nil
	}

	if util.ListContainsElement(*currentTraversalPaths, module.Path) {
		return errors.New(DependencyCycleError(append(*currentTraversalPaths, module.Path)))
	}

	*currentTraversalPaths = append(*currentTraversalPaths, module.Path)
	for _, dependency := range module.Dependencies {
		if err := checkForCyclesUsingDepthFirstSearch(dependency, visitedPaths, currentTraversalPaths); err != nil {
			return err
		}
	}

	*visitedPaths = append(*visitedPaths, module.Path)
	*currentTraversalPaths = util.RemoveElementFromList(*currentTraversalPaths, module.Path)

	return nil
}

// String renders this stack as a human-readable string
func (stack *Stack) String() string {
	var modules []string
	for _, module := range stack.Units {
		modules = append(modules, "  => "+module.String())
	}

	sort.Strings(modules)

	return fmt.Sprintf("Stack at %s:\n%s", stack.TerragruntOptions.WorkingDir, strings.Join(modules, "\n"))
}

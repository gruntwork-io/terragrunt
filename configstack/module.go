package configstack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/terraform"

	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const maxLevelsOfRecursion = 20
const existingModulesCacheName = "existingModules"

// Represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type TerraformModule struct {
	Path                 string
	Dependencies         TerraformModules
	Config               config.TerragruntConfig
	TerragruntOptions    *options.TerragruntOptions
	AssumeAlreadyApplied bool
	FlagExcluded         bool
}

// Render this module as a human-readable string
func (module *TerraformModule) String() string {
	dependencies := []string{}
	for _, dependency := range module.Dependencies {
		dependencies = append(dependencies, dependency.Path)
	}
	return fmt.Sprintf(
		"Module %s (excluded: %v, assume applied: %v, dependencies: [%s])",
		module.Path, module.FlagExcluded, module.AssumeAlreadyApplied, strings.Join(dependencies, ", "),
	)
}

func (module *TerraformModule) MarshalJSON() ([]byte, error) {
	return json.Marshal(module.Path)
}

// Check for cycles using a depth-first-search as described here:
// https://en.wikipedia.org/wiki/Topological_sorting#Depth-first_search
//
// Note that this method uses two lists, visitedPaths, and currentTraversalPaths, to track what nodes have already been
// seen. We need to use lists to maintain ordering so we can show the proper order of paths in a cycle. Of course, a
// list doesn't perform well with repeated contains() and remove() checks, so ideally we'd use an ordered Map (e.g.
// Java's LinkedHashMap), but since Go doesn't have such a data structure built-in, and our lists are going to be very
// small (at most, a few dozen paths), there is no point in worrying about performance.
func (module *TerraformModule) checkForCyclesUsingDepthFirstSearch(visitedPaths *[]string, currentTraversalPaths *[]string) error {
	if util.ListContainsElement(*visitedPaths, module.Path) {
		return nil
	}

	if util.ListContainsElement(*currentTraversalPaths, module.Path) {
		return errors.WithStackTrace(DependencyCycleError(append(*currentTraversalPaths, module.Path)))
	}

	*currentTraversalPaths = append(*currentTraversalPaths, module.Path)
	for _, dependency := range module.Dependencies {
		if err := dependency.checkForCyclesUsingDepthFirstSearch(visitedPaths, currentTraversalPaths); err != nil {
			return err
		}
	}

	*visitedPaths = append(*visitedPaths, module.Path)
	*currentTraversalPaths = util.RemoveElementFromList(*currentTraversalPaths, module.Path)

	return nil
}

// planFile - return plan file location, if output folder is set
func (module *TerraformModule) planFile(terragruntOptions *options.TerragruntOptions) string {
	planFile := ""

	// set plan file location if output folder is set
	planFile = module.outputFile(terragruntOptions)

	planCommand := module.TerragruntOptions.TerraformCommand == terraform.CommandNamePlan || module.TerragruntOptions.TerraformCommand == terraform.CommandNameShow

	// in case if JSON output is enabled, and not specified planFile, save plan in working dir
	if planCommand && planFile == "" && module.TerragruntOptions.JsonOutputFolder != "" {
		planFile = terraform.TerraformPlanFile
	}
	return planFile
}

// outputFile - return plan file location, if output folder is set
func (module *TerraformModule) outputFile(opts *options.TerragruntOptions) string {
	planFile := ""
	if opts.OutputFolder != "" {
		path, _ := filepath.Rel(opts.WorkingDir, module.Path)
		dir := filepath.Join(opts.OutputFolder, path)
		planFile = filepath.Join(dir, terraform.TerraformPlanFile)
	}
	return planFile
}

// outputJsonFile - return plan JSON file location, if JSON output folder is set
func (module *TerraformModule) outputJsonFile(opts *options.TerragruntOptions) string {
	jsonPlanFile := ""
	if opts.JsonOutputFolder != "" {
		path, _ := filepath.Rel(opts.WorkingDir, module.Path)
		dir := filepath.Join(opts.JsonOutputFolder, path)
		jsonPlanFile = filepath.Join(dir, terraform.TerraformPlanJsonFile)
	}
	return jsonPlanFile
}

// findModuleInPath returns true if a module is located under one of the target directories
func (module *TerraformModule) findModuleInPath(targetDirs []string) bool {
	for _, targetDir := range targetDirs {
		if module.Path == targetDir {
			return true
		}
	}
	return false
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given module is already
// applied. If the user selects "yes", then Terragrunt will apply that module as well.
// Note that we skip the prompt for `run-all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --terragrunt-include-external-dependencies or --terragrunt-include-dir flags.
func (module *TerraformModule) confirmShouldApplyExternalDependency(dependency *TerraformModule, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if terragruntOptions.IncludeExternalDependencies {
		terragruntOptions.Logger.Debugf("The --terragrunt-include-external-dependencies flag is set, so automatically including all external dependencies, and will run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return true, nil
	}

	if terragruntOptions.NonInteractive {
		terragruntOptions.Logger.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run-all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return false, nil
	}

	stackCmd := terragruntOptions.TerraformCommand
	if stackCmd == "destroy" {
		terragruntOptions.Logger.Debugf("run-all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run-all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return false, nil
	}

	prompt := fmt.Sprintf("Module: \t\t %s\nExternal dependency: \t %s\nShould Terragrunt apply the external dependency?", module.Path, dependency.Path)
	return shell.PromptUserForYesNo(prompt, terragruntOptions)
}

// Get the list of modules this module depends on
func (module *TerraformModule) getDependenciesForModule(modulesMap TerraformModulesMap, terragruntConfigPaths []string) (TerraformModules, error) {
	dependencies := TerraformModules{}

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
			return dependencies, errors.WithStackTrace(err)
		}
		dependencies = append(dependencies, dependencyModule)
	}

	return dependencies, nil
}

type TerraformModules []*TerraformModule

// FindWhereWorkingDirIsIncluded - find where working directory is included, flow:
// 1. Find root git top level directory and build list of modules
// 2. Iterate over includes from terragruntOptions if git top level directory detection failed
// 3. Filter found module only items which has in dependencies working directory
func FindWhereWorkingDirIsIncluded(ctx context.Context, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) TerraformModules {
	var pathsToCheck []string
	var matchedModulesMap = make(TerraformModulesMap)

	if gitTopLevelDir, err := shell.GitTopLevelDir(ctx, terragruntOptions, terragruntOptions.WorkingDir); err == nil {
		pathsToCheck = append(pathsToCheck, gitTopLevelDir)
	} else {
		// detection failed, trying to use include directories as source for stacks
		uniquePaths := make(map[string]bool)
		for _, includePath := range terragruntConfig.ProcessedIncludes {
			uniquePaths[filepath.Dir(includePath.Path)] = true
		}
		for path := range uniquePaths {
			pathsToCheck = append(pathsToCheck, path)
		}
	}

	for _, dir := range pathsToCheck { // iterate over detected paths, build stacks and filter modules by working dir
		dir += filepath.FromSlash("/")
		cfgOptions, err := options.NewTerragruntOptionsWithConfigPath(dir)
		if err != nil {
			terragruntOptions.Logger.Debugf("Failed to build terragrunt options from %s %v", dir, err)
			continue
		}

		cfgOptions.Env = terragruntOptions.Env
		cfgOptions.LogLevel = terragruntOptions.LogLevel
		cfgOptions.OriginalTerragruntConfigPath = terragruntOptions.OriginalTerragruntConfigPath
		cfgOptions.TerraformCommand = terragruntOptions.TerraformCommand
		cfgOptions.NonInteractive = true

		var hook = NewForceLogLevelHook(logrus.DebugLevel)
		cfgOptions.Logger.Logger.AddHook(hook)

		// build stack from config directory
		stack, err := FindStackInSubfolders(ctx, cfgOptions, WithChildTerragruntConfig(terragruntConfig))
		if err != nil {
			// log error as debug since in some cases stack building may fail because parent files can be designed
			// to work with relative paths from downstream modules
			terragruntOptions.Logger.Debugf("Failed to build module stack %v", err)
			continue
		}

		dependentModules := stack.ListStackDependentModules()
		deps, found := dependentModules[terragruntOptions.WorkingDir]
		if found {
			for _, module := range stack.Modules {
				for _, dep := range deps {
					if dep == module.Path {
						matchedModulesMap[module.Path] = module
						break
					}
				}
			}
		}
	}

	// extract modules as list
	var matchedModules = make(TerraformModules, 0, len(matchedModulesMap))
	for _, module := range matchedModulesMap {
		matchedModules = append(matchedModules, module)
	}

	return matchedModules
}

// WriteDot is used to emit a GraphViz compatible definition
// for a directed graph. It can be used to dump a .dot file.
// This is a similar implementation to terraform's digraph https://github.com/hashicorp/terraform/blob/master/digraph/graphviz.go
// adding some styling to modules that are excluded from the execution in *-all commands
func (modules TerraformModules) WriteDot(w io.Writer, terragruntOptions *options.TerragruntOptions) error {
	_, err := w.Write([]byte("digraph {\n"))
	if err != nil {
		return errors.WithStackTrace(err)
	}
	defer func(w io.Writer, p []byte) {
		_, err := w.Write(p)
		if err != nil {
			terragruntOptions.Logger.Warnf("Failed to close graphviz output: %v", err)
		}
	}(w, []byte("}\n"))

	// all paths are relative to the TerragruntConfigPath
	prefix := filepath.Dir(terragruntOptions.TerragruntConfigPath) + "/"

	for _, source := range modules {
		// apply a different coloring for excluded nodes
		style := ""
		if source.FlagExcluded {
			style = "[color=red]"
		}

		nodeLine := fmt.Sprintf("\t\"%s\" %s;\n",
			strings.TrimPrefix(source.Path, prefix), style)

		_, err := w.Write([]byte(nodeLine))
		if err != nil {
			return errors.WithStackTrace(err)
		}

		for _, target := range source.Dependencies {
			line := fmt.Sprintf("\t\"%s\" -> \"%s\";\n",
				strings.TrimPrefix(source.Path, prefix),
				strings.TrimPrefix(target.Path, prefix),
			)
			_, err := w.Write([]byte(line))
			if err != nil {
				return errors.WithStackTrace(err)
			}
		}
	}

	return nil
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (modules TerraformModules) RunModules(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	runningModules, err := modules.ToRunningModules(NormalOrder)
	if err != nil {
		return err
	}
	return runningModules.runModules(ctx, opts, parallelism)
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func (modules TerraformModules) RunModulesReverseOrder(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	runningModules, err := modules.ToRunningModules(ReverseOrder)
	if err != nil {
		return err
	}
	return runningModules.runModules(ctx, opts, parallelism)
}

// Run the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed without caring for inter-dependencies.
func (modules TerraformModules) RunModulesIgnoreOrder(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	runningModules, err := modules.ToRunningModules(IgnoreOrder)
	if err != nil {
		return err
	}
	return runningModules.runModules(ctx, opts, parallelism)
}

// Convert the list of modules to a map from module path to a runningModule struct. This struct contains information
// about executing the module, such as whether it has finished running or not and any errors that happened. Note that
// this does NOT actually run the module. For that, see the RunModules method.
func (modules TerraformModules) ToRunningModules(dependencyOrder DependencyOrder) (RunningModules, error) {
	runningModules := RunningModules{}
	for _, module := range modules {
		runningModules[module.Path] = newRunningModule(module)
	}

	crossLinkedModules, err := runningModules.crossLinkDependencies(dependencyOrder)
	if err != nil {
		return crossLinkedModules, err
	}

	return crossLinkedModules.RemoveFlagExcluded(), nil
}

// Check for dependency cycles in the given list of modules and return an error if one is found
func (modules TerraformModules) CheckForCycles() error {
	visitedPaths := []string{}
	currentTraversalPaths := []string{}

	for _, module := range modules {
		err := module.checkForCyclesUsingDepthFirstSearch(&visitedPaths, &currentTraversalPaths)
		if err != nil {
			return err
		}
	}

	return nil
}

// flagExcludedDirs iterates over a module slice and flags all entries as excluded, which should be ignored via the terragrunt-exclude-dir CLI flag.
func (modules TerraformModules) flagExcludedDirs(terragruntOptions *options.TerragruntOptions) TerraformModules {
	for _, module := range modules {
		if module.findModuleInPath(terragruntOptions.ExcludeDirs) {
			// Mark module itself as excluded
			module.FlagExcluded = true
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range module.Dependencies {
			if dependency.findModuleInPath(terragruntOptions.ExcludeDirs) {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules
}

// flagIncludedDirs iterates over a module slice and flags all entries not in the list specified via the terragrunt-include-dir CLI flag as excluded.
func (modules TerraformModules) flagIncludedDirs(terragruntOptions *options.TerragruntOptions) TerraformModules {
	// If we're not excluding by default, we should include everything by default.
	// This can happen when a user doesn't set include flags.
	if !terragruntOptions.ExcludeByDefault {
		// If we aren't given any include directories, but are given the strict include flag,
		// return no modules.
		if terragruntOptions.StrictInclude {
			return TerraformModules{}
		}
		return modules
	}

	for _, module := range modules {
		if module.findModuleInPath(terragruntOptions.IncludeDirs) {
			module.FlagExcluded = false
		} else {
			module.FlagExcluded = true
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !terragruntOptions.StrictInclude {
		for _, module := range modules {
			if !module.FlagExcluded {
				for _, dependency := range module.Dependencies {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules
}

// flagModulesThatDontInclude iterates over a module slice and flags all modules that don't include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute. Flagged modules will be filtered
// out of the set.
func (modules TerraformModules) flagModulesThatDontInclude(terragruntOptions *options.TerragruntOptions) (TerraformModules, error) {
	// If no ModulesThatInclude is specified return the modules list instantly
	if len(terragruntOptions.ModulesThatInclude) == 0 {
		return modules, nil
	}

	modulesThatIncludeCanonicalPath := []string{}
	for _, includePath := range terragruntOptions.ModulesThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, terragruntOptions.WorkingDir)
		if err != nil {
			return nil, err
		}

		modulesThatIncludeCanonicalPath = append(modulesThatIncludeCanonicalPath, canonicalPath)
	}

	for _, module := range modules {
		// Ignore modules that are already excluded because this feature is a filter for excluding the subset, not
		// including modules that have already been excluded through other means.
		if module.FlagExcluded {
			continue
		}

		// Mark modules that don't include any of the specified paths as excluded. To do this, we first flag the module
		// as excluded, and if it includes any path in the set, we set the exclude flag back to false.
		module.FlagExcluded = true
		for _, includeConfig := range module.Config.ProcessedIncludes {
			// resolve include config to canonical path to compare with modulesThatIncludeCanonicalPath
			// https://github.com/gruntwork-io/terragrunt/issues/1944
			canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
			if err != nil {
				return nil, err
			}
			if util.ListContainsElement(modulesThatIncludeCanonicalPath, canonicalPath) {
				module.FlagExcluded = false
			}
		}

		// Also search module dependencies and exclude if the dependency path doesn't include any of the specified
		// paths, using a similar logic.
		for _, dependency := range module.Dependencies {
			if dependency.FlagExcluded {
				continue
			}

			dependency.FlagExcluded = true
			for _, includeConfig := range dependency.Config.ProcessedIncludes {
				canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
				if err != nil {
					return nil, err
				}
				if util.ListContainsElement(modulesThatIncludeCanonicalPath, canonicalPath) {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules, nil
}

var existingModules = cache.NewCache[*TerraformModulesMap](existingModulesCacheName)

type TerraformModulesMap map[string]*TerraformModule

// Merge the given external dependencies into the given map of modules if those dependencies aren't already in the
// modules map
func (modulesMap TerraformModulesMap) mergeMaps(externalDependencies TerraformModulesMap) TerraformModulesMap {
	out := TerraformModulesMap{}

	for key, value := range externalDependencies {
		out[key] = value
	}

	for key, value := range modulesMap {
		out[key] = value
	}

	return out
}

// Go through each module in the given map and cross-link its dependencies to the other modules in that same map. If
// a dependency is referenced that is not in the given map, return an error.
func (modulesMap TerraformModulesMap) crosslinkDependencies(canonicalTerragruntConfigPaths []string) (TerraformModules, error) {
	modules := TerraformModules{}

	keys := modulesMap.getSortedKeys()
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

// Return the keys for the given map in sorted order. This is used to ensure we always iterate over maps of modules
// in a consistent order (Go does not guarantee iteration order for maps, and usually makes it random)
func (modulesMap TerraformModulesMap) getSortedKeys() []string {
	keys := []string{}
	for key := range modulesMap {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

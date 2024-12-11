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
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const maxLevelsOfRecursion = 20
const existingModulesCacheName = "existingModules"

// TerraformModule represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type TerraformModule struct {
	*Stack
	Path                 string
	Dependencies         TerraformModules
	Config               config.TerragruntConfig
	TerragruntOptions    *options.TerragruntOptions
	AssumeAlreadyApplied bool
	FlagExcluded         bool
}

// String renders this module as a human-readable string
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

// FlushOutput flushes buffer data to the output writer.
func (module *TerraformModule) FlushOutput() error {
	if writer, ok := module.TerragruntOptions.Writer.(*ModuleWriter); ok {
		module.outputMu.Lock()
		defer module.outputMu.Unlock()

		return writer.Flush()
	}

	return nil
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
		return errors.New(DependencyCycleError(append(*currentTraversalPaths, module.Path)))
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
func (module *TerraformModule) planFile(opts *options.TerragruntOptions) string {
	var planFile string

	// set plan file location if output folder is set
	planFile = module.outputFile(opts)

	planCommand := module.TerragruntOptions.TerraformCommand == terraform.CommandNamePlan || module.TerragruntOptions.TerraformCommand == terraform.CommandNameShow

	// in case if JSON output is enabled, and not specified planFile, save plan in working dir
	if planCommand && planFile == "" && module.TerragruntOptions.JSONOutputFolder != "" {
		planFile = terraform.TerraformPlanFile
	}

	return planFile
}

// outputFile - return plan file location, if output folder is set
func (module *TerraformModule) outputFile(opts *options.TerragruntOptions) string {
	return module.getPlanFilePath(opts, opts.OutputFolder, terraform.TerraformPlanFile)
}

// outputJSONFile - return plan JSON file location, if JSON output folder is set
func (module *TerraformModule) outputJSONFile(opts *options.TerragruntOptions) string {
	return module.getPlanFilePath(opts, opts.JSONOutputFolder, terraform.TerraformPlanJSONFile)
}

func (module *TerraformModule) getPlanFilePath(opts *options.TerragruntOptions, outputFolder, fileName string) string {
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
			opts.Logger.Warnf("Failed to get absolute path for %s: %v", dir, err)
		}
	}

	return filepath.Join(dir, fileName)
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
func (module *TerraformModule) confirmShouldApplyExternalDependency(ctx context.Context, dependency *TerraformModule, opts *options.TerragruntOptions) (bool, error) {
	if opts.IncludeExternalDependencies {
		opts.Logger.Debugf("The --terragrunt-include-external-dependencies flag is set, so automatically including all external dependencies, and will run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return true, nil
	}

	if opts.NonInteractive {
		opts.Logger.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run-all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return false, nil
	}

	stackCmd := opts.TerraformCommand
	if stackCmd == "destroy" {
		opts.Logger.Debugf("run-all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run-all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
		return false, nil
	}

	opts.Logger.Infof("Module %s has external dependency %s", module.Path, dependency.Path)

	return shell.PromptUserForYesNo(ctx, "Should Terragrunt apply the external dependency?", opts)
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

			return dependencies, errors.New(err)
		}

		dependencies = append(dependencies, dependencyModule)
	}

	return dependencies, nil
}

type TerraformModules []*TerraformModule

// FindWhereWorkingDirIsIncluded - find where working directory is included, flow:
// 1. Find root git top level directory and build list of modules
// 2. Iterate over includes from opts if git top level directory detection failed
// 3. Filter found module only items which has in dependencies working directory
func FindWhereWorkingDirIsIncluded(ctx context.Context, opts *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) TerraformModules {
	var (
		pathsToCheck      []string
		matchedModulesMap = make(TerraformModulesMap)
	)

	if gitTopLevelDir, err := shell.GitTopLevelDir(ctx, opts, opts.WorkingDir); err == nil {
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
			opts.Logger.Debugf("Failed to build terragrunt options from %s %v", dir, err)
			continue
		}

		cfgOptions.Env = opts.Env
		cfgOptions.LogLevel = opts.LogLevel
		cfgOptions.OriginalTerragruntConfigPath = opts.OriginalTerragruntConfigPath
		cfgOptions.TerraformCommand = opts.TerraformCommand
		cfgOptions.NonInteractive = true
		cfgOptions.Logger = cfgOptions.Logger.WithOptions(log.WithHooks(NewForceLogLevelHook(log.DebugLevel)))

		// build stack from config directory
		stack, err := FindStackInSubfolders(ctx, cfgOptions, WithChildTerragruntConfig(terragruntConfig))
		if err != nil {
			// log error as debug since in some cases stack building may fail because parent files can be designed
			// to work with relative paths from downstream modules
			opts.Logger.Debugf("Failed to build module stack %v", err)
			continue
		}

		dependentModules := stack.ListStackDependentModules()

		deps, found := dependentModules[opts.WorkingDir]
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
func (modules TerraformModules) WriteDot(w io.Writer, opts *options.TerragruntOptions) error {
	if _, err := w.Write([]byte("digraph {\n")); err != nil {
		return errors.New(err)
	}
	defer func(w io.Writer, p []byte) {
		_, err := w.Write(p)
		if err != nil {
			opts.Logger.Warnf("Failed to close graphviz output: %v", err)
		}
	}(w, []byte("}\n"))

	// all paths are relative to the TerragruntConfigPath
	prefix := filepath.Dir(opts.TerragruntConfigPath) + "/"

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

// RunModules runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in an order determined by their inter-dependencies, using
// as much concurrency as possible.
func (modules TerraformModules) RunModules(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	runningModules, err := modules.ToRunningModules(NormalOrder)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, parallelism)
}

// RunModulesReverseOrder runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed in the reverse order of their inter-dependencies, using
// as much concurrency as possible.
func (modules TerraformModules) RunModulesReverseOrder(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	runningModules, err := modules.ToRunningModules(ReverseOrder)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, parallelism)
}

// RunModulesIgnoreOrder runs the given map of module path to runningModule. To "run" a module, execute the RunTerragrunt command in its
// TerragruntOptions object. The modules will be executed without caring for inter-dependencies.
func (modules TerraformModules) RunModulesIgnoreOrder(ctx context.Context, opts *options.TerragruntOptions, parallelism int) error {
	runningModules, err := modules.ToRunningModules(IgnoreOrder)
	if err != nil {
		return err
	}

	return runningModules.runModules(ctx, opts, parallelism)
}

// ToRunningModules converts the list of modules to a map from module path to a runningModule struct. This struct contains information
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

// CheckForCycles checks for dependency cycles in the given list of modules and return an error if one is found.
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

// flagIncludedDirs includes all units by default.
//
// However, when anything that triggers ExcludeByDefault is set, the function will instead
// selectively include only the units that are in the list specified via the IncludeDirs option.
func (modules TerraformModules) flagIncludedDirs(opts *options.TerragruntOptions) TerraformModules {
	if !opts.ExcludeByDefault {
		return modules
	}

	for _, module := range modules {
		if module.findModuleInPath(opts.IncludeDirs) {
			module.FlagExcluded = false
		} else {
			module.FlagExcluded = true
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
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

// flagUnitsThatAreIncluded iterates over a module slice and flags all modules that include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute.
func (modules TerraformModules) flagUnitsThatAreIncluded(opts *options.TerragruntOptions) (TerraformModules, error) {
	// The two flags ModulesThatInclude and UnitsReading should both be considered when determining which
	// units to include in the run queue.
	unitsThatInclude := append(opts.ModulesThatInclude, opts.UnitsReading...)

	// If no unitsThatInclude is specified return the modules list instantly
	if len(unitsThatInclude) == 0 {
		return modules, nil
	}

	modulesThatIncludeCanonicalPaths := []string{}

	for _, includePath := range unitsThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		modulesThatIncludeCanonicalPaths = append(modulesThatIncludeCanonicalPaths, canonicalPath)
	}

	for _, module := range modules {
		for _, includeConfig := range module.Config.ProcessedIncludes {
			// resolve include config to canonical path to compare with modulesThatIncludeCanonicalPath
			// https://github.com/gruntwork-io/terragrunt/issues/1944
			canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
			if err != nil {
				return nil, err
			}

			if util.ListContainsElement(modulesThatIncludeCanonicalPaths, canonicalPath) {
				module.FlagExcluded = false
			}
		}

		// Also search module dependencies and exclude if the dependency path doesn't include any of the specified
		// paths, using a similar logic.
		for _, dependency := range module.Dependencies {
			if dependency.FlagExcluded {
				continue
			}

			for _, includeConfig := range dependency.Config.ProcessedIncludes {
				canonicalPath, err := util.CanonicalPath(includeConfig.Path, module.Path)
				if err != nil {
					return nil, err
				}

				if util.ListContainsElement(modulesThatIncludeCanonicalPaths, canonicalPath) {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return modules, nil
}

// flagExcludedUnits iterates over a module slice and flags all modules that are excluded based on the exclude block.
func (modules TerraformModules) flagExcludedUnits(opts *options.TerragruntOptions) TerraformModules {
	for _, module := range modules {
		excludeConfig := module.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			opts.Logger.Debugf("Module %s is excluded by exclude block", module.Path)
			module.FlagExcluded = true
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			opts.Logger.Debugf("Excluding dependencies for module %s by exclude block", module.Path)

			for _, dependency := range module.Dependencies {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules
}

// flagUnitsThatRead iterates over a module slice and flags all modules that read at least one file in the specified
// file list in the TerragruntOptions UnitsReading attribute.
func (modules TerraformModules) flagUnitsThatRead(opts *options.TerragruntOptions) (TerraformModules, error) {
	// If no UnitsThatRead is specified return the modules list instantly
	if len(opts.UnitsReading) == 0 {
		return modules, nil
	}

	for _, readPath := range opts.UnitsReading {
		path, err := util.CanonicalPath(readPath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		for _, module := range modules {
			if opts.DidReadFile(path, module.Path) {
				module.FlagExcluded = false
			}
		}
	}

	return modules, nil
}

// flagExcludedDirs iterates over a module slice and flags all entries as excluded listed in the terragrunt-exclude-dir CLI flag.
func (modules TerraformModules) flagExcludedDirs(opts *options.TerragruntOptions) TerraformModules {
	// If we don't have any excludes, we don't need to do anything.
	if len(opts.ExcludeDirs) == 0 {
		return modules
	}

	for _, module := range modules {
		if module.findModuleInPath(opts.ExcludeDirs) {
			// Mark module itself as excluded
			module.FlagExcluded = true
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range module.Dependencies {
			if dependency.findModuleInPath(opts.ExcludeDirs) {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules
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

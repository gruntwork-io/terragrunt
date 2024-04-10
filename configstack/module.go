package configstack

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/telemetry"

	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
)

const maxLevelsOfRecursion = 20

// Represents a single module (i.e. folder with Terraform templates), including the Terragrunt configuration for that
// module and the list of other modules that this module depends on
type TerraformModule struct {
	Path                 string
	Dependencies         []*TerraformModule
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

func (module TerraformModule) MarshalJSON() ([]byte, error) {
	return json.Marshal(module.Path)
}

// Go through each of the given Terragrunt configuration files and resolve the module that configuration file represents
// into a TerraformModule struct. Return the list of these TerraformModule structs.
func ResolveTerraformModules(ctx context.Context, terragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions, childTerragruntConfig *config.TerragruntConfig, howThesePathsWereFound string) ([]*TerraformModule, error) {
	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return nil, err
	}

	var modules map[string]*TerraformModule
	err = telemetry.Telemetry(ctx, terragruntOptions, "resolve_modules", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {

		result, err := resolveModules(ctx, canonicalTerragruntConfigPaths, terragruntOptions, childTerragruntConfig, howThesePathsWereFound)
		if err != nil {
			return err
		}
		modules = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	var externalDependencies map[string]*TerraformModule
	err = telemetry.Telemetry(ctx, terragruntOptions, "resolve_external_dependencies_for_modules", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := resolveExternalDependenciesForModules(ctx, modules, map[string]*TerraformModule{}, 0, terragruntOptions, childTerragruntConfig)
		if err != nil {
			return err
		}
		externalDependencies = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	var crossLinkedModules []*TerraformModule
	err = telemetry.Telemetry(ctx, terragruntOptions, "crosslink_dependencies", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := crosslinkDependencies(mergeMaps(modules, externalDependencies), canonicalTerragruntConfigPaths)
		if err != nil {
			return err
		}
		crossLinkedModules = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	var includedModules []*TerraformModule
	err = telemetry.Telemetry(ctx, terragruntOptions, "flag_included_dirs", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		includedModules = flagIncludedDirs(crossLinkedModules, terragruntOptions)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var includedModulesWithExcluded []*TerraformModule
	err = telemetry.Telemetry(ctx, terragruntOptions, "flag_excluded_dirs", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		includedModulesWithExcluded = flagExcludedDirs(includedModules, terragruntOptions)
		return nil
	})
	if err != nil {
		return nil, err
	}

	var finalModules []*TerraformModule
	err = telemetry.Telemetry(ctx, terragruntOptions, "flag_modules_that_dont_include", map[string]interface{}{
		"working_dir": terragruntOptions.WorkingDir,
	}, func(childCtx context.Context) error {
		result, err := flagModulesThatDontInclude(includedModulesWithExcluded, terragruntOptions)
		if err != nil {
			return err
		}
		finalModules = result
		return nil
	})
	if err != nil {
		return nil, err
	}

	return finalModules, nil
}

// flagExcludedDirs iterates over a module slice and flags all entries as excluded, which should be ignored via the terragrunt-exclude-dir CLI flag.
func flagExcludedDirs(modules []*TerraformModule, terragruntOptions *options.TerragruntOptions) []*TerraformModule {
	for _, module := range modules {
		if findModuleInPath(module, terragruntOptions.ExcludeDirs) {
			// Mark module itself as excluded
			module.FlagExcluded = true
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range module.Dependencies {
			if findModuleInPath(dependency, terragruntOptions.ExcludeDirs) {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules
}

// flagIncludedDirs iterates over a module slice and flags all entries not in the list specified via the terragrunt-include-dir CLI flag as excluded.
func flagIncludedDirs(modules []*TerraformModule, terragruntOptions *options.TerragruntOptions) []*TerraformModule {

	// If no IncludeDirs is specified return the modules list instantly
	if len(terragruntOptions.IncludeDirs) == 0 {
		// If we aren't given any include directories, but are given the strict include flag,
		// return no modules.
		if terragruntOptions.StrictInclude {
			return []*TerraformModule{}
		}
		return modules
	}

	for _, module := range modules {
		if findModuleInPath(module, terragruntOptions.IncludeDirs) {
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

// findModuleInPath returns true if a module is located under one of the target directories
func findModuleInPath(module *TerraformModule, targetDirs []string) bool {
	for _, targetDir := range targetDirs {
		if module.Path == targetDir {
			return true
		}
	}
	return false
}

// flagModulesThatDontInclude iterates over a module slice and flags all modules that don't include at least one file in
// the specified include list on the TerragruntOptions ModulesThatInclude attribute. Flagged modules will be filtered
// out of the set.
func flagModulesThatDontInclude(modules []*TerraformModule, terragruntOptions *options.TerragruntOptions) ([]*TerraformModule, error) {

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

// Go through each of the given Terragrunt configuration files and resolve the module that configuration file represents
// into a TerraformModule struct. Note that this method will NOT fill in the Dependencies field of the TerraformModule
// struct (see the crosslinkDependencies method for that). Return a map from module path to TerraformModule struct.
func resolveModules(ctx context.Context, canonicalTerragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions, childTerragruntConfig *config.TerragruntConfig, howTheseModulesWereFound string) (map[string]*TerraformModule, error) {
	moduleMap := map[string]*TerraformModule{}
	for _, terragruntConfigPath := range canonicalTerragruntConfigPaths {
		var module *TerraformModule
		err := telemetry.Telemetry(ctx, terragruntOptions, "resolve_terraform_module", map[string]interface{}{
			"config_path": terragruntConfigPath,
			"working_dir": terragruntOptions.WorkingDir,
		}, func(childCtx context.Context) error {
			m, err := resolveTerraformModule(terragruntConfigPath, moduleMap, terragruntOptions, childTerragruntConfig, howTheseModulesWereFound)
			if err != nil {
				return err
			}
			module = m
			return nil
		})
		if err != nil {
			return moduleMap, err
		}
		if module != nil {
			moduleMap[module.Path] = module
			var dependencies map[string]*TerraformModule
			err := telemetry.Telemetry(ctx, terragruntOptions, "resolve_dependencies_for_module", map[string]interface{}{
				"config_path": terragruntConfigPath,
				"working_dir": terragruntOptions.WorkingDir,
				"module_path": module.Path,
			}, func(childCtx context.Context) error {
				deps, err := resolveDependenciesForModule(ctx, module, moduleMap, terragruntOptions, childTerragruntConfig, true)
				if err != nil {
					return err
				}
				dependencies = deps
				return nil
			})
			if err != nil {
				return moduleMap, err
			}
			moduleMap = collections.MergeMaps(moduleMap, dependencies)
		}
	}

	return moduleMap, nil
}

// Create a TerraformModule struct for the Terraform module specified by the given Terragrunt configuration file path.
// Note that this method will NOT fill in the Dependencies field of the TerraformModule struct (see the
// crosslinkDependencies method for that).
func resolveTerraformModule(terragruntConfigPath string, moduleMap map[string]*TerraformModule, terragruntOptions *options.TerragruntOptions, childTerragruntConfig *config.TerragruntConfig, howThisModuleWasFound string) (*TerraformModule, error) {
	modulePath, err := util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
	if err != nil {
		return nil, err
	}

	if _, ok := moduleMap[modulePath]; ok {
		return nil, nil
	}

	// Clone the options struct so we don't modify the original one. This is especially important as run-all operations
	// happen concurrently.
	opts := terragruntOptions.Clone(terragruntConfigPath)

	// We need to reset the original path for each module. Otherwise, this path will be set to wherever you ran run-all
	// from, which is not what any of the modules will want.
	opts.OriginalTerragruntConfigPath = terragruntConfigPath

	// If `childTerragruntConfig.ProcessedIncludes` contains the path `terragruntConfigPath`, then this is a parent config
	// which implies that `TerragruntConfigPath` must refer to a child configuration file, and the defined `IncludeConfig` must contain the path to the file itself
	// for the built-in functions `read-terragrunt-config()`, `path_relative_to_include()` to work correctly.
	var includeConfig *config.IncludeConfig
	if childTerragruntConfig != nil && childTerragruntConfig.ProcessedIncludes.ContainsPath(terragruntConfigPath) {
		includeConfig = &config.IncludeConfig{Path: terragruntConfigPath}
		opts.TerragruntConfigPath = terragruntOptions.OriginalTerragruntConfigPath
	}

	if collections.ListContainsElement(opts.ExcludeDirs, modulePath) {
		// module is excluded
		return &TerraformModule{Path: modulePath, TerragruntOptions: opts, FlagExcluded: true}, nil
	}

	configContext := config.NewParsingContext(context.Background(), opts).WithDecodeList(
		// Need for initializing the modules
		config.TerraformSource,

		// Need for parsing out the dependencies
		config.DependenciesBlock,
		config.DependencyBlock,
	)

	// We only partially parse the config, only using the pieces that we need in this section. This config will be fully
	// parsed at a later stage right before the action is run. This is to delay interpolation of functions until right
	// before we call out to terraform.
	terragruntConfig, err := config.PartialParseConfigFile(
		configContext,
		terragruntConfigPath,
		includeConfig,
	)
	if err != nil {
		return nil, errors.WithStackTrace(ErrorProcessingModule{UnderlyingError: err, HowThisModuleWasFound: howThisModuleWasFound, ModulePath: terragruntConfigPath})
	}

	terragruntSource, err := config.GetTerragruntSourceForModule(terragruntOptions.Source, modulePath, terragruntConfig)
	if err != nil {
		return nil, err
	}
	opts.Source = terragruntSource

	_, defaultDownloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntOptions.TerragruntConfigPath)
	if err != nil {
		return nil, err
	}

	// If we're using the default download directory, put it into the same folder as the Terragrunt configuration file.
	// If we're not using the default, then the user has specified a custom download directory, and we leave it as-is.
	if terragruntOptions.DownloadDir == defaultDownloadDir {
		_, downloadDir, err := options.DefaultWorkingAndDownloadDirs(terragruntConfigPath)
		if err != nil {
			return nil, err
		}
		terragruntOptions.Logger.Debugf("Setting download directory for module %s to %s", modulePath, downloadDir)
		opts.DownloadDir = downloadDir
	}

	// Fix for https://github.com/gruntwork-io/terragrunt/issues/208
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(terragruntConfigPath), "*.tf"))
	if err != nil {
		return nil, err
	}
	if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && matches == nil {
		terragruntOptions.Logger.Debugf("Module %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
		return nil, nil
	}

	if opts.IncludeModulePrefix {
		opts.OutputPrefix = fmt.Sprintf("[%v] ", modulePath)
	}

	return &TerraformModule{Path: modulePath, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

// Look through the dependencies of the modules in the given map and resolve the "external" dependency paths listed in
// each modules config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to apply-all or destroy-all. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the TerraformModule struct (see the crosslinkDependencies method for that).
func resolveExternalDependenciesForModules(ctx context.Context, moduleMap map[string]*TerraformModule, modulesAlreadyProcessed map[string]*TerraformModule, recursionLevel int, terragruntOptions *options.TerragruntOptions, childTerragruntConfig *config.TerragruntConfig) (map[string]*TerraformModule, error) {
	allExternalDependencies := map[string]*TerraformModule{}
	modulesToSkip := mergeMaps(moduleMap, modulesAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.WithStackTrace(InfiniteRecursion{RecursionLevel: maxLevelsOfRecursion, Modules: modulesToSkip})
	}

	sortedKeys := getSortedKeys(moduleMap)
	for _, key := range sortedKeys {
		module := moduleMap[key]
		externalDependencies, err := resolveDependenciesForModule(ctx, module, modulesToSkip, terragruntOptions, childTerragruntConfig, false)
		if err != nil {
			return externalDependencies, err
		}

		for _, externalDependency := range externalDependencies {
			if _, alreadyFound := modulesToSkip[externalDependency.Path]; alreadyFound {
				continue
			}

			shouldApply := false
			if !terragruntOptions.IgnoreExternalDependencies {
				shouldApply, err = confirmShouldApplyExternalDependency(module, externalDependency, terragruntOptions)
				if err != nil {
					return externalDependencies, err
				}
			}

			externalDependency.AssumeAlreadyApplied = !shouldApply
			allExternalDependencies[externalDependency.Path] = externalDependency
		}
	}

	if len(allExternalDependencies) > 0 {
		recursiveDependencies, err := resolveExternalDependenciesForModules(ctx, allExternalDependencies, moduleMap, recursionLevel+1, terragruntOptions, childTerragruntConfig)
		if err != nil {
			return allExternalDependencies, err
		}
		return mergeMaps(allExternalDependencies, recursiveDependencies), nil
	}

	return allExternalDependencies, nil
}

var existingModules = config.NewCache[*map[string]*TerraformModule]()

// resolveDependenciesForModule looks through the dependencies of the given module and resolve the dependency paths listed in the module's config.
// If `skipExternal` is true, the func returns only dependencies that are inside of the current working directory, which means they are part of the environment the
// user is trying to apply-all or destroy-all. Note that this method will NOT fill in the Dependencies field of the TerraformModule struct (see the crosslinkDependencies method for that).
func resolveDependenciesForModule(ctx context.Context, module *TerraformModule, moduleMap map[string]*TerraformModule, terragruntOptions *options.TerragruntOptions, chilTerragruntConfig *config.TerragruntConfig, skipExternal bool) (map[string]*TerraformModule, error) {
	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return map[string]*TerraformModule{}, nil
	}

	key := fmt.Sprintf("%s-%s-%v-%v", module.Path, terragruntOptions.WorkingDir, skipExternal, terragruntOptions.TerraformCommand)
	if value, ok := existingModules.Get(key); ok {
		return *value, nil
	}

	externalTerragruntConfigPaths := []string{}
	for _, dependency := range module.Config.Dependencies.Paths {
		dependencyPath, err := util.CanonicalPath(dependency, module.Path)
		if err != nil {
			return map[string]*TerraformModule{}, err
		}

		if skipExternal && !util.HasPathPrefix(dependencyPath, terragruntOptions.WorkingDir) {
			continue
		}

		terragruntConfigPath := config.GetDefaultConfigPath(dependencyPath)

		if _, alreadyContainsModule := moduleMap[dependencyPath]; !alreadyContainsModule {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of module at '%s'", module.Path)
	result, err := resolveModules(ctx, externalTerragruntConfigPaths, terragruntOptions, chilTerragruntConfig, howThesePathsWereFound)
	if err != nil {
		return nil, err
	}

	existingModules.Put(key, &result)
	return result, nil
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given module is already
// applied. If the user selects "yes", then Terragrunt will apply that module as well.
// Note that we skip the prompt for `run-all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --terragrunt-include-external-dependencies or --terragrunt-include-dir flags.
func confirmShouldApplyExternalDependency(module *TerraformModule, dependency *TerraformModule, terragruntOptions *options.TerragruntOptions) (bool, error) {
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

// Merge the given external dependencies into the given map of modules if those dependencies aren't already in the
// modules map
func mergeMaps(modules map[string]*TerraformModule, externalDependencies map[string]*TerraformModule) map[string]*TerraformModule {
	out := map[string]*TerraformModule{}

	for key, value := range externalDependencies {
		out[key] = value
	}

	for key, value := range modules {
		out[key] = value
	}

	return out
}

// Go through each module in the given map and cross-link its dependencies to the other modules in that same map. If
// a dependency is referenced that is not in the given map, return an error.
func crosslinkDependencies(moduleMap map[string]*TerraformModule, canonicalTerragruntConfigPaths []string) ([]*TerraformModule, error) {
	modules := []*TerraformModule{}

	keys := getSortedKeys(moduleMap)
	for _, key := range keys {
		module := moduleMap[key]
		dependencies, err := getDependenciesForModule(module, moduleMap, canonicalTerragruntConfigPaths)
		if err != nil {
			return modules, err
		}

		module.Dependencies = dependencies
		modules = append(modules, module)
	}

	return modules, nil
}

// Get the list of modules this module depends on
func getDependenciesForModule(module *TerraformModule, moduleMap map[string]*TerraformModule, terragruntConfigPaths []string) ([]*TerraformModule, error) {
	dependencies := []*TerraformModule{}

	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range module.Config.Dependencies.Paths {
		dependencyModulePath, err := util.CanonicalPath(dependencyPath, module.Path)
		if err != nil {
			return dependencies, nil
		}

		if files.FileExists(dependencyModulePath) && !files.IsDir(dependencyModulePath) {
			dependencyModulePath = filepath.Dir(dependencyModulePath)
		}

		dependencyModule, foundModule := moduleMap[dependencyModulePath]
		if !foundModule {
			err := UnrecognizedDependency{
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

// Return the keys for the given map in sorted order. This is used to ensure we always iterate over maps of modules
// in a consistent order (Go does not guarantee iteration order for maps, and usually makes it random)
func getSortedKeys(modules map[string]*TerraformModule) []string {
	keys := []string{}
	for key := range modules {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

// FindWhereWorkingDirIsIncluded - find where working directory is included, flow:
// 1. Find root git top level directory and build list of modules
// 2. Iterate over includes from terragruntOptions if git top level directory detection failed
// 3. Filter found module only items which has in dependencies working directory
func FindWhereWorkingDirIsIncluded(ctx context.Context, terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []*TerraformModule {
	var pathsToCheck []string
	var matchedModulesMap = make(map[string]*TerraformModule)

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
		stack, err := FindStackInSubfolders(ctx, cfgOptions, terragruntConfig)
		if err != nil {
			// log error as debug since in some cases stack building may fail because parent files can be designed
			// to work with relative paths from downstream modules
			terragruntOptions.Logger.Debugf("Failed to build module stack %v", err)
			continue
		}

		dependentModules := ListStackDependentModules(stack)
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
	var matchedModules []*TerraformModule
	for _, module := range matchedModulesMap {
		matchedModules = append(matchedModules, module)
	}

	return matchedModules
}

// ForceLogLevelHook - log hook which can change log level for messages which contains specific substrings
type ForceLogLevelHook struct {
	TriggerLevels []logrus.Level
	ForcedLevel   logrus.Level
}

// NewForceLogLevelHook - create default log reduction hook
func NewForceLogLevelHook(forcedLevel logrus.Level) *ForceLogLevelHook {
	return &ForceLogLevelHook{
		ForcedLevel:   forcedLevel,
		TriggerLevels: logrus.AllLevels,
	}
}

// Levels - return log levels on which hook will be triggered
func (hook *ForceLogLevelHook) Levels() []logrus.Level {
	return hook.TriggerLevels
}

// Fire - function invoked against log entries when entry will match loglevel from Levels()
func (hook *ForceLogLevelHook) Fire(entry *logrus.Entry) error {
	entry.Level = hook.ForcedLevel
	// special formatter to skip printing of log entries since after hook evaluation, entries are printed directly
	formatter := LogEntriesDropperFormatter{OriginalFormatter: entry.Logger.Formatter}
	entry.Logger.Formatter = &formatter
	return nil
}

// LogEntriesDropperFormatter - custom formatter which will ignore log entries which has lower level than preconfigured in logger
type LogEntriesDropperFormatter struct {
	OriginalFormatter logrus.Formatter
}

// Format - custom entry formatting function which will drop entries with lower level than set in logger
func (formatter *LogEntriesDropperFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Logger.Level >= entry.Level {
		return formatter.OriginalFormatter.Format(entry)
	}
	return []byte(""), nil
}

// ListStackDependentModules - build a map with each module and its dependent modules
func ListStackDependentModules(stack *Stack) map[string][]string {
	// build map of dependent modules
	// module path -> list of dependent modules
	var dependentModules = make(map[string][]string)

	// build initial mapping of dependent modules
	for _, module := range stack.Modules {

		if len(module.Dependencies) != 0 {
			for _, dep := range module.Dependencies {
				dependentModules[dep.Path] = util.RemoveDuplicatesFromList(append(dependentModules[dep.Path], module.Path))
			}
		}
	}

	// Floyd–Warshall inspired approach to find dependent modules
	// merge map slices by key until no more updates are possible

	// Example:
	// Initial setup:
	// dependentModules["module1"] = ["module2", "module3"]
	// dependentModules["module2"] = ["module3"]
	// dependentModules["module3"] = ["module4"]
	// dependentModules["module4"] = ["module5"]

	// After first iteration: (module1 += module4, module2 += module4, module3 += module5)
	// dependentModules["module1"] = ["module2", "module3", "module4"]
	// dependentModules["module2"] = ["module3", "module4"]
	// dependentModules["module3"] = ["module4", "module5"]
	// dependentModules["module4"] = ["module5"]

	// After second iteration: (module1 += module5, module2 += module5)
	// dependentModules["module1"] = ["module2", "module3", "module4", "module5"]
	// dependentModules["module2"] = ["module3", "module4", "module5"]
	// dependentModules["module3"] = ["module4", "module5"]
	// dependentModules["module4"] = ["module5"]

	// Done, no more updates and in map we have all dependent modules for each module.

	for {
		noUpdates := true
		for module, dependents := range dependentModules {
			for _, dependent := range dependents {
				initialSize := len(dependentModules[module])
				// merge without duplicates
				list := util.RemoveDuplicatesFromList(append(dependentModules[module], dependentModules[dependent]...))
				list = util.RemoveElementFromList(list, module)
				dependentModules[module] = list
				if initialSize != len(dependentModules[module]) {
					noUpdates = false
				}
			}
		}
		if noUpdates {
			break
		}
	}
	return dependentModules
}

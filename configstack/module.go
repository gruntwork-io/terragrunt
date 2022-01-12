package configstack

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	zglob "github.com/mattn/go-zglob"
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

// Go through each of the given Terragrunt configuration files and resolve the module that configuration file represents
// into a TerraformModule struct. Return the list of these TerraformModule structs.
func ResolveTerraformModules(terragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions, howThesePathsWereFound string) ([]*TerraformModule, error) {
	canonicalTerragruntConfigPaths, err := util.CanonicalPaths(terragruntConfigPaths, ".")
	if err != nil {
		return []*TerraformModule{}, err
	}

	modules, err := resolveModules(canonicalTerragruntConfigPaths, terragruntOptions, howThesePathsWereFound)
	if err != nil {
		return []*TerraformModule{}, err
	}

	externalDependencies, err := resolveExternalDependenciesForModules(modules, map[string]*TerraformModule{}, 0, terragruntOptions)
	if err != nil {
		return []*TerraformModule{}, err
	}

	crossLinkedModules, err := crosslinkDependencies(mergeMaps(modules, externalDependencies), canonicalTerragruntConfigPaths)
	if err != nil {
		return []*TerraformModule{}, err
	}

	includedModules, err := flagIncludedDirs(crossLinkedModules, terragruntOptions)
	if err != nil {
		return []*TerraformModule{}, err
	}

	includedModulesWithExcluded, err := flagExcludedDirs(includedModules, terragruntOptions)
	if err != nil {
		return []*TerraformModule{}, err
	}

	finalModules, err := flagModulesThatDontInclude(includedModulesWithExcluded, terragruntOptions)
	if err != nil {
		return []*TerraformModule{}, err
	}

	return finalModules, nil
}

//flagExcludedDirs iterates over a module slice and flags all entries as excluded, which should be ignored via the terragrunt-exclude-dir CLI flag.
func flagExcludedDirs(modules []*TerraformModule, terragruntOptions *options.TerragruntOptions) ([]*TerraformModule, error) {

	// If no ExcludeDirs is specified return the modules list instantly
	if len(terragruntOptions.ExcludeDirs) == 0 {
		return modules, nil
	}

	canonicalWorkingDir, err := util.CanonicalPath("", terragruntOptions.WorkingDir)
	if err != nil {
		return nil, err
	}

	excludeGlobMatches := []string{}

	// If possible, expand the glob to get all excluded filepaths
	for _, dir := range terragruntOptions.ExcludeDirs {

		absoluteDir := ""

		// Ensure excludedDirs are absolute
		if filepath.IsAbs(dir) {
			absoluteDir = dir
		} else {
			absoluteDir = filepath.Join(canonicalWorkingDir, dir)
		}

		matches, err := zglob.Glob(absoluteDir)

		// Skip globs that can not be expanded
		if err == nil {
			excludeGlobMatches = append(excludeGlobMatches, matches...)
		}
	}

	// Make sure all paths are canonical
	canonicalExcludeDirs := []string{}
	for _, module := range excludeGlobMatches {
		canonicalPath, err := util.CanonicalPath(module, terragruntOptions.WorkingDir)
		if err != nil {
			return nil, err
		}
		canonicalExcludeDirs = append(canonicalExcludeDirs, canonicalPath)
	}

	for _, module := range modules {
		if findModuleinPath(module, canonicalExcludeDirs) {
			// Mark module itself as excluded
			module.FlagExcluded = true
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range module.Dependencies {
			if findModuleinPath(dependency, canonicalExcludeDirs) {
				dependency.FlagExcluded = true
			}
		}
	}

	return modules, nil
}

//flagIncludedDirs iterates over a module slice and flags all entries not in the list specified via the terragrunt-include-dir CLI flag  as excluded.
func flagIncludedDirs(modules []*TerraformModule, terragruntOptions *options.TerragruntOptions) ([]*TerraformModule, error) {

	// If no IncludeDirs is specified return the modules list instantly
	if len(terragruntOptions.IncludeDirs) == 0 {
		// If we aren't given any include directories, but are given the strict include flag,
		// return no modules.
		if terragruntOptions.StrictInclude {
			return []*TerraformModule{}, nil
		}
		return modules, nil
	}

	canonicalWorkingDir, err := util.CanonicalPath("", terragruntOptions.WorkingDir)
	if err != nil {
		return nil, err
	}

	includeGlobMatches := []string{}

	// If possible, expand the glob to get all included filepaths
	for _, dir := range terragruntOptions.IncludeDirs {

		absoluteDir := dir

		// Ensure includedDirs are absolute
		if !filepath.IsAbs(dir) {
			absoluteDir = filepath.Join(canonicalWorkingDir, dir)
		}

		matches, err := zglob.Glob(absoluteDir)

		// Skip globs that can not be expanded
		if err == nil {
			includeGlobMatches = append(includeGlobMatches, matches...)
		}
	}

	// Make sure all paths are canonical
	canonicalIncludeDirs := []string{}
	for _, module := range includeGlobMatches {
		canonicalPath, err := util.CanonicalPath(module, terragruntOptions.WorkingDir)
		if err != nil {
			return nil, err
		}
		canonicalIncludeDirs = append(canonicalIncludeDirs, canonicalPath)
	}

	for _, module := range modules {
		if findModuleinPath(module, canonicalIncludeDirs) {
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

	return modules, nil
}

// Returns true if a module is located under one of the target directories
func findModuleinPath(module *TerraformModule, targetDirs []string) bool {
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
func resolveModules(canonicalTerragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions, howTheseModulesWereFound string) (map[string]*TerraformModule, error) {
	moduleMap := map[string]*TerraformModule{}

	for _, terragruntConfigPath := range canonicalTerragruntConfigPaths {
		module, err := resolveTerraformModule(terragruntConfigPath, terragruntOptions, howTheseModulesWereFound)
		if err != nil {
			return moduleMap, err
		}
		if module != nil {
			moduleMap[module.Path] = module
		}
	}

	return moduleMap, nil
}

// Create a TerraformModule struct for the Terraform module specified by the given Terragrunt configuration file path.
// Note that this method will NOT fill in the Dependencies field of the TerraformModule struct (see the
// crosslinkDependencies method for that).
func resolveTerraformModule(terragruntConfigPath string, terragruntOptions *options.TerragruntOptions, howThisModuleWasFound string) (*TerraformModule, error) {
	modulePath, err := util.CanonicalPath(filepath.Dir(terragruntConfigPath), ".")
	if err != nil {
		return nil, err
	}

	// Clone the options struct so we don't modify the original one. This is especially important as run-all operations
	// happen concurrently.
	opts := terragruntOptions.Clone(terragruntConfigPath)

	// We need to reset the original path for each module. Otherwise, this path will be set to wherever you ran run-all
	// from, which is not what any of the modules will want.
	opts.OriginalTerragruntConfigPath = terragruntConfigPath

	// We only partially parse the config, only using the pieces that we need in this section. This config will be fully
	// parsed at a later stage right before the action is run. This is to delay interpolation of functions until right
	// before we call out to terraform.
	terragruntConfig, err := config.PartialParseConfigFile(
		terragruntConfigPath,
		opts,
		nil,
		[]config.PartialDecodeSectionType{
			// Need for initializing the modules
			config.TerraformSource,

			// Need for parsing out the dependencies
			config.DependenciesBlock,
			config.DependencyBlock,
		},
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

	return &TerraformModule{Path: modulePath, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

// Look through the dependencies of the modules in the given map and resolve the "external" dependency paths listed in
// each modules config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to apply-all or destroy-all. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the TerraformModule struct (see the crosslinkDependencies method for that).
func resolveExternalDependenciesForModules(moduleMap map[string]*TerraformModule, modulesAlreadyProcessed map[string]*TerraformModule, recursionLevel int, terragruntOptions *options.TerragruntOptions) (map[string]*TerraformModule, error) {
	allExternalDependencies := map[string]*TerraformModule{}
	modulesToSkip := mergeMaps(moduleMap, modulesAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.WithStackTrace(InfiniteRecursion{RecursionLevel: maxLevelsOfRecursion, Modules: modulesToSkip})
	}

	sortedKeys := getSortedKeys(moduleMap)
	for _, key := range sortedKeys {
		module := moduleMap[key]
		externalDependencies, err := resolveExternalDependenciesForModule(module, modulesToSkip, terragruntOptions)
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
		recursiveDependencies, err := resolveExternalDependenciesForModules(allExternalDependencies, moduleMap, recursionLevel+1, terragruntOptions)
		if err != nil {
			return allExternalDependencies, err
		}
		return mergeMaps(allExternalDependencies, recursiveDependencies), nil
	}

	return allExternalDependencies, nil
}

// Look through the dependencies of the given module and resolve the "external" dependency paths listed in the module's
// config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths). These external
// dependencies are outside of the current working directory, which means they may not be part of the environment the
// user is trying to apply-all or destroy-all. Note that this method will NOT fill in the Dependencies field of the
// TerraformModule struct (see the crosslinkDependencies method for that).
func resolveExternalDependenciesForModule(module *TerraformModule, moduleMap map[string]*TerraformModule, terragruntOptions *options.TerragruntOptions) (map[string]*TerraformModule, error) {
	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return map[string]*TerraformModule{}, nil
	}

	externalTerragruntConfigPaths := []string{}
	for _, dependency := range module.Config.Dependencies.Paths {
		dependencyPath, err := util.CanonicalPath(dependency, module.Path)
		if err != nil {
			return map[string]*TerraformModule{}, err
		}

		terragruntConfigPath := config.GetDefaultConfigPath(dependencyPath)
		if _, alreadyContainsModule := moduleMap[dependencyPath]; !alreadyContainsModule {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of module at '%s'", module.Path)
	return resolveModules(externalTerragruntConfigPaths, terragruntOptions, howThesePathsWereFound)
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

	prompt := fmt.Sprintf("Module %s depends on module %s, which is an external dependency outside of the current working directory. Should Terragrunt run this external dependency? Warning, if you say 'yes', Terragrunt will make changes in %s as well!", module.Path, dependency.Path, dependency.Path)
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
func FindWhereWorkingDirIsIncluded(terragruntOptions *options.TerragruntOptions, terragruntConfig *config.TerragruntConfig) []*TerraformModule {
	var pathsToCheck []string
	var matchedModulesMap = make(map[string]*TerraformModule)
	var gitTopLevelDir = ""
	gitTopLevelDir, err := shell.GitTopLevelDir(terragruntOptions, terragruntOptions.WorkingDir)
	if err == nil { // top level detection worked
		pathsToCheck = append(pathsToCheck, gitTopLevelDir)
	} else { // detection failed, trying to use include directories as source for stacks
		uniquePaths := make(map[string]bool)
		for _, includePath := range terragruntConfig.ProcessedIncludes {
			uniquePaths[filepath.Dir(includePath.Path)] = true
		}
		for path := range uniquePaths {
			pathsToCheck = append(pathsToCheck, path)
		}
	}
	for _, dir := range pathsToCheck { // iterate over detected paths, build stacks and filter modules by working dir
		dir = dir + filepath.FromSlash("/")
		cfgOptions, err := options.NewTerragruntOptions(dir)
		if err != nil {
			terragruntOptions.Logger.Debugf("Failed to build terragrunt options from %s %v", dir, err)
			return nil
		}
		cfgOptions.LogLevel = terragruntOptions.LogLevel
		if terragruntOptions.TerraformCommand == "destroy" {
			var hook = NewForceLogLevelHook(logrus.DebugLevel)
			cfgOptions.Logger.Logger.AddHook(hook)
		}
		stack, err := FindStackInSubfolders(cfgOptions)
		if err != nil {
			// loggign error as debug since in some cases stack building may fail because parent files can be designed
			// to work with relative paths from downstream modules
			terragruntOptions.Logger.Debugf("Failed to build module stack %v", err)
			return nil
		}

		for _, module := range stack.Modules {
			for _, dep := range module.Dependencies {
				if dep.Path == terragruntOptions.WorkingDir { // include in dependencies module which have in dependencies WorkingDir
					matchedModulesMap[module.Path] = module
					break
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

// Custom error types

type UnrecognizedDependency struct {
	ModulePath            string
	DependencyPath        string
	TerragruntConfigPaths []string
}

func (err UnrecognizedDependency) Error() string {
	return fmt.Sprintf("Module %s specifies %s as a dependency, but that dependency was not one of the ones found while scanning subfolders: %v", err.ModulePath, err.DependencyPath, err.TerragruntConfigPaths)
}

type ErrorProcessingModule struct {
	UnderlyingError       error
	ModulePath            string
	HowThisModuleWasFound string
}

func (err ErrorProcessingModule) Error() string {
	return fmt.Sprintf("Error processing module at '%s'. How this module was found: %s. Underlying error: %v", err.ModulePath, err.HowThisModuleWasFound, err.UnderlyingError)
}

type InfiniteRecursion struct {
	RecursionLevel int
	Modules        map[string]*TerraformModule
}

func (err InfiniteRecursion) Error() string {
	return fmt.Sprintf("Hit what seems to be an infinite recursion after going %d levels deep. Please check for a circular dependency! Modules involved: %v", err.RecursionLevel, err.Modules)
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

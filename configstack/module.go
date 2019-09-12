package configstack

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/go-getter"
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
	return fmt.Sprintf("Module %s (excluded: %v, dependencies: [%s])", module.Path, module.FlagExcluded, strings.Join(dependencies, ", "))
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

	finalModules, err := flagExcludedDirs(includedModules, terragruntOptions)
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
			// Mark module itself as included
			module.FlagExcluded = false
			// Mark all affected dependencies as included
			for _, dependency := range module.Dependencies {
				dependency.FlagExcluded = false
			}
		} else {
			module.FlagExcluded = true
		}
	}

	return modules, nil
}

// Returns true if a module is located under one of the target directories
func findModuleinPath(module *TerraformModule, targetDirs []string) bool {
	for _, targetDir := range targetDirs {
		if strings.Contains(module.Path, targetDir) {
			return true
		}
	}
	return false
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

	opts := terragruntOptions.Clone(terragruntConfigPath)
	// We only partially parse the config, only using the pieces that we need in this section. This config will be fully
	// parsed at a later stage right before the action is run. This is to delay interpolation of functions until right
	// before we call out to terraform.
	terragruntConfig, err := config.PartialParseConfigFile(
		terragruntConfigPath,
		opts,
		nil,
		[]config.PartialDecodeSectionType{
			// Need for initializing the modules
			config.TerraformBlock,

			// Need for parsing out the dependencies
			config.DependenciesBlock,
			config.DependencyBlock,
		},
	)
	if err != nil {
		return nil, errors.WithStackTrace(ErrorProcessingModule{UnderlyingError: err, HowThisModuleWasFound: howThisModuleWasFound, ModulePath: terragruntConfigPath})
	}

	terragruntSource, err := getTerragruntSourceForModule(modulePath, terragruntConfig, terragruntOptions)
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
		terragruntOptions.Logger.Printf("Setting download directory for module %s to %s", modulePath, downloadDir)
		opts.DownloadDir = downloadDir
	}

	// Fix for https://github.com/gruntwork-io/terragrunt/issues/208
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(terragruntConfigPath), "*.tf"))
	if err != nil {
		return nil, err
	}
	if (terragruntConfig.Terraform == nil || terragruntConfig.Terraform.Source == nil || *terragruntConfig.Terraform.Source == "") && matches == nil {
		terragruntOptions.Logger.Printf("Module %s does not have an associated terraform configuration and will be skipped.", filepath.Dir(terragruntConfigPath))
		return nil, nil
	}

	return &TerraformModule{Path: modulePath, Config: *terragruntConfig, TerragruntOptions: opts}, nil
}

// If one of the xxx-all commands is called with the --terragrunt-source parameter, then for each module, we need to
// build its own --terragrunt-source parameter by doing the following:
//
// 1. Read the source URL from the Terragrunt configuration of each module
// 2. Extract the path from that URL (the part after a double-slash)
// 3. Append the path to the --terragrunt-source parameter
//
// Example:
//
// --terragrunt-source: /source/infrastructure-modules
// source param in module's terragrunt.hcl: git::git@github.com:acme/infrastructure-modules.git//networking/vpc?ref=v0.0.1
//
// This method will return: /source/infrastructure-modules//networking/vpc
//
func getTerragruntSourceForModule(modulePath string, moduleTerragruntConfig *config.TerragruntConfig, terragruntOptions *options.TerragruntOptions) (string, error) {
	if terragruntOptions.Source == "" || moduleTerragruntConfig.Terraform == nil || moduleTerragruntConfig.Terraform.Source == nil || *moduleTerragruntConfig.Terraform.Source == "" {
		return "", nil
	}

	// use go-getter to split the module source string into a valid URL and subdirectory (if // is present)
	moduleUrl, moduleSubdir := getter.SourceDirSubdir(*moduleTerragruntConfig.Terraform.Source)

	// if both URL and subdir are missing, something went terribly wrong
	if moduleUrl == "" && moduleSubdir == "" {
		return "", errors.WithStackTrace(InvalidSourceUrl{ModulePath: modulePath, ModuleSourceUrl: *moduleTerragruntConfig.Terraform.Source, TerragruntSource: terragruntOptions.Source})
	}
	// if only subdir is missing, check if we can obtain a valid module name from the URL portion
	if moduleUrl != "" && moduleSubdir == "" {
		moduleSubdirFromUrl, err := getModulePathFromSourceUrl(moduleUrl)
		if err != nil {
			return moduleSubdirFromUrl, err
		}
		return util.JoinTerraformModulePath(terragruntOptions.Source, moduleSubdirFromUrl), nil
	}

	return util.JoinTerraformModulePath(terragruntOptions.Source, moduleSubdir), nil
}

// Parse sourceUrl not containing '//', and attempt to obtain a module path.
// Example:
//
// sourceUrl = "git::ssh://git@ghe.ourcorp.com/OurOrg/module-name.git"
// will return "module-name".

func getModulePathFromSourceUrl(sourceUrl string) (string, error) {

	// Regexp for module name extraction. It assumes that the query string has already been stripped off.
	// Then we simply capture anything after the last slash, and before `.` or end of string.
	var moduleNameRegexp = regexp.MustCompile(`(?:.+/)(.+?)(?:\.|$)`)

	// strip off the query string if present
	sourceUrl = strings.Split(sourceUrl, "?")[0]

	matches := moduleNameRegexp.FindStringSubmatch(sourceUrl)

	// if regexp returns less/more than the full match + 1 capture group, then something went wrong with regex (invalid source string)
	if len(matches) != 2 {
		return "", errors.WithStackTrace(ErrorParsingModulePath{ModuleSourceUrl: sourceUrl})
	}

	return matches[1], nil
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

		terragruntConfigPath := config.DefaultConfigPath(dependencyPath)
		if _, alreadyContainsModule := moduleMap[dependencyPath]; !alreadyContainsModule {
			externalTerragruntConfigPaths = append(externalTerragruntConfigPaths, terragruntConfigPath)
		}
	}

	howThesePathsWereFound := fmt.Sprintf("dependency of module at '%s'", module.Path)
	return resolveModules(externalTerragruntConfigPaths, terragruntOptions, howThesePathsWereFound)
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given module is already
// applied. If the user selects "yes", then Terragrunt will apply that module as well.
func confirmShouldApplyExternalDependency(module *TerraformModule, dependency *TerraformModule, terragruntOptions *options.TerragruntOptions) (bool, error) {
	if terragruntOptions.NonInteractive {
		terragruntOptions.Logger.Printf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with an xxx-all command, will not run this command against module %s, which is a dependency of module %s.", dependency.Path, module.Path)
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
	for key, _ := range modules {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
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

type InvalidSourceUrl struct {
	ModulePath       string
	ModuleSourceUrl  string
	TerragruntSource string
}

func (err InvalidSourceUrl) Error() string {
	return fmt.Sprintf("The --terragrunt-source parameter is set to '%s', but the source URL in the module at '%s' is invalid: '%s'. Note that the module URL must have a double-slash to separate the repo URL from the path within the repo!", err.TerragruntSource, err.ModulePath, err.ModuleSourceUrl)
}

type ErrorParsingModulePath struct {
	ModuleSourceUrl string
}

func (err ErrorParsingModulePath) Error() string {
	return fmt.Sprintf("Unable to obtain the module path from the source URL '%s'. Ensure that the URL is in a supported format.", err.ModuleSourceUrl)
}

type InfiniteRecursion struct {
	RecursionLevel int
	Modules        map[string]*TerraformModule
}

func (err InfiniteRecursion) Error() string {
	return fmt.Sprintf("Hit what seems to be an infinite recursion after going %d levels deep. Please check for a circular dependency! Modules involved: %v", err.RecursionLevel, err.Modules)
}

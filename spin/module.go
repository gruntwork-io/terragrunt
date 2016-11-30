package spin

import (
	"github.com/gruntwork-io/terragrunt/config"
	"path/filepath"
	"fmt"
	"github.com/gruntwork-io/terragrunt/errors"
	"strings"
	"github.com/gruntwork-io/terragrunt/options"
)

// Represents a single module (i.e. folder with Terraform templates), including the .terragrunt config for that module
// and the list of other modules that this module depends on
type TerraformModule struct {
	Path              string
	Dependencies      []*TerraformModule
	Config            config.TerragruntConfig
	TerragruntOptions *options.TerragruntOptions
}

// Render this module as a human-readable string
func (module *TerraformModule) String() string {
	dependencies := []string{}
	for _, dependency := range module.Dependencies {
		dependencies = append(dependencies, dependency.Path)
	}
	return fmt.Sprintf("Module %s (dependencies: [%s])", module.Path, strings.Join(dependencies, ", "))
}

// Go through each of the given .terragrunt config files and resolve the Terraform module that .terragrunt file
// represents, including its dependencies on other Terraform Modules.
func ResolveTerraformModules(terragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions) ([]*TerraformModule, error) {
	modules := []*TerraformModule{}

	moduleMap, err := createModuleMap(terragruntConfigPaths, terragruntOptions)
	if err != nil {
		return modules, err
	}

	for _, module := range moduleMap {
		dependencies, err := getDependenciesForModule(module, moduleMap, terragruntConfigPaths)
		if err != nil {
			return modules, err
		}

		module.Dependencies = dependencies
		modules = append(modules, module)
	}

	return modules, nil
}

// Create a mapping from the absolute path of a module to the TerraformModule struct for that module. Note that this
// method will NOT fill in the Dependencies field of the TerraformModule struct.
func createModuleMap(terragruntConfigPaths []string, terragruntOptions *options.TerragruntOptions) (map[string]*TerraformModule, error) {
	moduleMap := map[string]*TerraformModule{}

	for _, terragruntConfigPath := range terragruntConfigPaths {
		opts := terragruntOptions.Clone(terragruntConfigPath)
		terragruntConfig, err := config.ParseConfigFile(terragruntConfigPath, opts, nil)
		if err != nil {
			return moduleMap, err
		}

		modulePath, err := resolvePathForModule(terragruntConfigPath)
		if err != nil {
			return moduleMap, err
		}

		moduleMap[modulePath] = &TerraformModule{
			Path: modulePath,
			Config: *terragruntConfig,
			TerragruntOptions: opts,
		}
	}

	return moduleMap, nil
}

// Get the list of modules this module depends on
func getDependenciesForModule(module *TerraformModule, moduleMap map[string]*TerraformModule, terragruntConfigPaths []string) ([]*TerraformModule, error) {
	dependencies := []*TerraformModule{}

	if module.Config.Dependencies == nil || len(module.Config.Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range module.Config.Dependencies.Paths {
		dependencyModulePath := resolvePathForDependency(dependencyPath, module.Path)
		dependencyModule, foundModule := moduleMap[dependencyModulePath]
		if !foundModule {
			err := UnrecognizedDependency{
				ModulePath: module.Path,
				DependencyPath: dependencyPath,
				TerragruntConfigPaths: terragruntConfigPaths,
			}
			return dependencies, errors.WithStackTrace(err)
		}
		dependencies = append(dependencies, dependencyModule)
	}

	return dependencies, nil
}

// Return the path for the Terraform module represented by the given .terragrunt config file
func resolvePathForModule(terragruntConfigPath string) (string, error) {
	modulePath, err := filepath.Abs(filepath.Dir(terragruntConfigPath))
	if err != nil {
		return "", errors.WithStackTrace(err)
	}
	return modulePath, nil
}

// Return the path for the given dependency, which is specified in a .terragrunt config file of the given module
func resolvePathForDependency(dependencyPath string, modulePath string) string {
	if filepath.IsAbs(dependencyPath) {
		return dependencyPath
	}

	return filepath.Join(modulePath, dependencyPath)
}

// Custom error types

type UnrecognizedDependency struct {
	ModulePath              string
	DependencyPath          string
	TerragruntConfigPaths   []string
}

func (err UnrecognizedDependency) Error() string {
	return fmt.Sprintf("Module %s specifies %s as a dependency, but that dependency was not one of the ones found while scanning subfolders: %v", err.ModulePath, err.DependencyPath, err.TerragruntConfigPaths)
}
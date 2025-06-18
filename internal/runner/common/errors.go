package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"
)

// Custom error types

type UnrecognizedDependencyError struct {
	ModulePath            string
	DependencyPath        string
	TerragruntConfigPaths []string
}

func (err UnrecognizedDependencyError) Error() string {
	return fmt.Sprintf("Module %s specifies %s as a dependency, but that dependency was not one of the ones found while scanning subfolders: %v", err.ModulePath, err.DependencyPath, err.TerragruntConfigPaths)
}

type ProcessingModuleError struct {
	UnderlyingError       error
	ModulePath            string
	HowThisModuleWasFound string
}

func (err ProcessingModuleError) Error() string {
	return fmt.Sprintf("Error processing module at '%s'. How this module was found: %s. Underlying error: %v", err.ModulePath, err.HowThisModuleWasFound, err.UnderlyingError)
}

func (err ProcessingModuleError) Unwrap() error {
	return err.UnderlyingError
}

type InfiniteRecursionError struct {
	Modules        map[string]*Unit
	RecursionLevel int
}

func (err InfiniteRecursionError) Error() string {
	return fmt.Sprintf("Hit what seems to be an infinite recursion after going %d levels deep. Please check for a circular dependency! Units involved: %v", err.RecursionLevel, err.Modules)
}

var ErrNoTerraformModulesFound = errors.New("could not find any subfolders with Terragrunt configuration files")

type DependencyCycleError []string

func (err DependencyCycleError) Error() string {
	return "Found a dependency cycle between modules: " + strings.Join([]string(err), " -> ")
}

type ProcessingModuleDependencyError struct {
	Module     *Unit
	Dependency *Unit
	Err        error
}

func (err ProcessingModuleDependencyError) Error() string {
	return fmt.Sprintf("Cannot process module %s because one of its dependencies, %s, finished with an error: %s", err.Module, err.Dependency, err.Err)
}

func (err ProcessingModuleDependencyError) ExitStatus() (int, error) {
	if exitCode, err := util.GetExitCode(err.Err); err == nil {
		return exitCode, nil
	}

	return -1, err
}

func (err ProcessingModuleDependencyError) Unwrap() error {
	return err.Err
}

type DependencyNotFoundWhileCrossLinkingError struct {
	Module     *Unit
	Dependency *Unit
}

func (err DependencyNotFoundWhileCrossLinkingError) Error() string {
	return fmt.Sprintf("Unit %v specifies a dependency on unit %v, but could not find that module while cross-linking dependencies. This is most likely a bug in Terragrunt. Please report it.", err.Module, err.Dependency)
}

package runbase

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gruntwork-io/terragrunt/util"
)

// Custom error types

type UnrecognizedDependencyError struct {
	UnitPath              string
	DependencyPath        string
	TerragruntConfigPaths []string
}

func (err UnrecognizedDependencyError) Error() string {
	return fmt.Sprintf("Unit %s specifies %s as a dependency, but that dependency was not one of the ones found while scanning subfolders: %v", err.UnitPath, err.DependencyPath, err.TerragruntConfigPaths)
}

type ProcessingUnitError struct {
	UnderlyingError     error
	UnitPath            string
	HowThisUnitWasFound string
}

func (err ProcessingUnitError) Error() string {
	return fmt.Sprintf("Error processing unit at '%s'. How this unit was found: %s. Underlying error: %v", err.UnitPath, err.HowThisUnitWasFound, err.UnderlyingError)
}

func (err ProcessingUnitError) Unwrap() error {
	return err.UnderlyingError
}

type InfiniteRecursionError struct {
	Units          map[string]*Unit
	RecursionLevel int
}

func (err InfiniteRecursionError) Error() string {
	return fmt.Sprintf("Hit what seems to be an infinite recursion after going %d levels deep. Please check for a circular dependency! Units involved: %v", err.RecursionLevel, err.Units)
}

var ErrNoUnitsFound = errors.New("could not find any subfolders with Terragrunt configuration files")

type DependencyCycleError []string

func (err DependencyCycleError) Error() string {
	return "Found a dependency cycle between modules: " + strings.Join([]string(err), " -> ")
}

type ProcessingUnitDependencyError struct {
	Unit       *Unit
	Dependency *Unit
	Err        error
}

func (err ProcessingUnitDependencyError) Error() string {
	return fmt.Sprintf("Cannot process module %s because one of its dependencies, %s, finished with an error: %s", err.Unit, err.Dependency, err.Err)
}

func (err ProcessingUnitDependencyError) ExitStatus() (int, error) {
	if exitCode, err := util.GetExitCode(err.Err); err == nil {
		return exitCode, nil
	}

	return -1, err
}

func (err ProcessingUnitDependencyError) Unwrap() error {
	return err.Err
}

type DependencyNotFoundWhileCrossLinkingError struct {
	Module     *Unit
	Dependency *Unit
}

func (err DependencyNotFoundWhileCrossLinkingError) Error() string {
	return fmt.Sprintf("Unit %v specifies a dependency on unit %v, but could not find that module while cross-linking dependencies. This is most likely a bug in Terragrunt. Please report it.", err.Module, err.Dependency)
}

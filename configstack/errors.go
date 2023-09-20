package configstack

import "fmt"

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

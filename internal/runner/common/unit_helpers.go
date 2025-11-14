package common

import (
	"path/filepath"

	"github.com/gruntwork-io/go-commons/files"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/util"
)

// ConvertDiscoveryToRunner converts units from discovery domain to runner domain by resolving
// Component interface dependencies into concrete *Unit pointer dependencies.
// Discovery found all dependencies and stored them as Component interfaces, but runner needs
// concrete *Unit pointers for efficient execution. This function translates between domains.
func ConvertDiscoveryToRunner(unitsMap component.UnitsMap, canonicalTerragruntConfigPaths []string) (component.Units, error) {
	units := component.Units{}

	keys := unitsMap.SortedKeys()

	for _, key := range keys {
		unit := unitsMap[key]

		dependencies, err := getDependenciesForUnit(unit, unitsMap, canonicalTerragruntConfigPaths)
		if err != nil {
			return units, err
		}

		// Set the concrete dependencies slice
		// Note: This modifies the unit's dependencies field directly
		// The component.Unit.dependencies field contains Components from discovery
		// We need to add a method to set concrete dependencies for runner
		for _, dep := range dependencies {
			unit.AddDependency(dep)
		}

		units = append(units, unit)
	}

	return units, nil
}

// getDependenciesForUnit gets the list of units this unit depends on.
func getDependenciesForUnit(unit *component.Unit, unitsMap component.UnitsMap, terragruntConfigPaths []string) (component.Units, error) {
	dependencies := component.Units{}

	cfg := unit.Config()
	if cfg == nil || cfg.Dependencies == nil || len(cfg.Dependencies.Paths) == 0 {
		return dependencies, nil
	}

	for _, dependencyPath := range cfg.Dependencies.Paths {
		dependencyUnitPath, err := util.CanonicalPath(dependencyPath, unit.Path())
		if err != nil {
			return dependencies, errors.Errorf("failed to resolve canonical path for dependency %s: %w", dependencyPath, err)
		}

		if files.FileExists(dependencyUnitPath) && !files.IsDir(dependencyUnitPath) {
			dependencyUnitPath = filepath.Dir(dependencyUnitPath)
		}

		dependencyUnit, foundUnit := unitsMap[dependencyUnitPath]
		if !foundUnit {
			dependencyErr := UnrecognizedDependencyError{
				UnitPath:              unit.Path(),
				DependencyPath:        dependencyPath,
				TerragruntConfigPaths: terragruntConfigPaths,
			}

			return dependencies, dependencyErr
		}

		dependencies = append(dependencies, dependencyUnit)
	}

	return dependencies, nil
}

// EnsureAbsolutePath ensures a path is absolute, converting it if necessary.
// Returns the absolute path and any error encountered during conversion.
func EnsureAbsolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", errors.Errorf("failed to get absolute path for %s: %w", path, err)
	}

	return absPath, nil
}

package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// GraphDependencyFilter filters units to include only a target directory and its dependents.
// This filter is used by the graph command to show only relevant units in the dependency graph.
type GraphDependencyFilter struct {
	// TargetDir is the directory whose dependents should be included
	TargetDir string
}

// Filter implements UnitFilter.
// It marks all units as excluded except for the target directory and units that depend on it.
func (f *GraphDependencyFilter) Filter(ctx context.Context, units Units, opts *options.TerragruntOptions) error {
	// Build dependency map first
	dependentUnits := make(map[string][]string)

	for _, unit := range units {
		if len(unit.Dependencies) != 0 {
			for _, dep := range unit.Dependencies {
				dependentUnits[dep.Path] = util.RemoveDuplicatesFromList(append(dependentUnits[dep.Path], unit.Path))
			}
		}
	}

	// Recursively collect all dependent units
	// Use a safe upper bound to prevent infinite loops
	maxIterations := len(units)*len(units) + 1
	for i := 0; i < maxIterations; i++ {
		updated := false

		for unit, dependents := range dependentUnits {
			for _, dep := range dependents {
				old := dependentUnits[unit]
				newList := util.RemoveDuplicatesFromList(
					append(old, dependentUnits[dep]...),
				)
				newList = util.RemoveElementFromList(newList, unit)

				if len(newList) != len(old) {
					dependentUnits[unit] = newList
					updated = true
				}
			}
		}

		if !updated {
			break
		}
	}

	// Determine which modules to include
	modulesToInclude := dependentUnits[f.TargetDir]
	modulesToInclude = append(modulesToInclude, f.TargetDir)

	// Mark units as excluded unless they are in modulesToInclude
	for _, module := range units {
		module.FlagExcluded = true
		if util.ListContainsElement(modulesToInclude, module.Path) {
			module.FlagExcluded = false
		}
	}

	return nil
}

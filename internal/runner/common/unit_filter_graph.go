package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// UnitFilterGraph filters units to include only a target directory and its dependents.
// This filter is used by the graph command to show only relevant units in the dependency graph.
type UnitFilterGraph struct {
	// TargetDir is the directory whose dependents should be included
	TargetDir string
}

// Filter implements UnitFilter.
// It marks all units as excluded except for the target directory and units that depend on it.
func (f *UnitFilterGraph) Filter(ctx context.Context, units Units, opts *options.TerragruntOptions) error {
	// Normalize target directory path for consistent comparison
	targetDir := util.CleanPath(f.TargetDir)

	// Build dependency map first, using normalized paths
	dependentUnits := make(map[string][]string)

	for _, unit := range units {
		deps := unit.Dependencies()
		if len(deps) != 0 {
			unitPath := util.CleanPath(unit.Path())
			for _, dep := range deps {
				depPath := util.CleanPath(dep.Path())
				dependentUnits[depPath] = util.RemoveDuplicatesFromList(append(dependentUnits[depPath], unitPath))
			}
		}
	}

	// Propagate transitive dependencies across all units.
	// A DAG can have up to N−1 levels, so at most N iterations are needed.
	// Each iteration propagates one level deeper; exceeding N implies a cycle.
	// See: https://en.wikipedia.org/wiki/Topological_sorting#Properties
	maxIterations := len(units)
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
	modulesToInclude := dependentUnits[targetDir]
	if modulesToInclude == nil {
		modulesToInclude = []string{}
	}

	modulesToInclude = append(modulesToInclude, targetDir)

	// Mark units as excluded unless they are in modulesToInclude
	for _, unit := range units {
		unitPath := util.CleanPath(unit.Path())
		unit.SetFilterExcluded(true)

		if util.ListContainsElement(modulesToInclude, unitPath) {
			unit.SetFilterExcluded(false)
		}
	}

	return nil
}

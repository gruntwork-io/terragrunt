package common

import (
	"context"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
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
func (f *UnitFilterGraph) Filter(ctx context.Context, units component.Units, opts *options.TerragruntOptions) error {
	if len(units) == 0 {
		return nil
	}

	// Normalize target directory path for consistent comparison
	targetDir := util.CleanPath(f.TargetDir)

	var dependency *component.Unit

	for _, u := range units {
		if u.Path() == targetDir {
			dependency = u
			break
		}
	}

	for _, u := range units {
		u.SetFilterExcluded(true)

		if dependency == nil {
			continue
		}

		if u == dependency || isDependent(u, dependency) {
			u.SetFilterExcluded(false)
		}
	}

	return nil
}

// isDependent returns true if x is a dependent of y (including by transitive dependents).
func isDependent(x, y component.Component) bool {
	const maxIterations = 1000000 // Sensible upper bound for discovery.
	return isDependentBounded(x, y, maxIterations)
}

// isDependentBounded returns true if x is a dependent of y (including by transitive dependents).
// It returns false if the remaining iterations is less than 0.
func isDependentBounded(x, y component.Component, remaining int) bool {
	if remaining <= 0 {
		return false
	}

	dependents := y.Dependents()
	if len(dependents) == 0 {
		return false
	}

	if slices.Contains(dependents, x) {
		return true
	}

	for _, dependent := range dependents {
		if isDependentBounded(x, dependent, remaining-1) {
			return true
		}
	}

	return false
}

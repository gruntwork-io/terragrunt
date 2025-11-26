package runnerpool

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

// UnitFilter applies filtering logic to resolved units.
// Filters are applied after units are resolved but before the queue is built.
// They can modify the Execution.FlagExcluded field to control which units are included in execution.
type UnitFilter interface {
	// Filter applies the filtering logic to the given units.
	// Returns an error if the filtering operation fails.
	Filter(ctx context.Context, units []*component.Unit, opts *options.TerragruntOptions) error
}

// CompositeFilter combines multiple filters into a single filter.
// Filters are applied in the order they are provided.
type CompositeFilter struct {
	Filters []UnitFilter
}

// Filter implements UnitFilter by applying all filters in sequence.
func (f *CompositeFilter) Filter(ctx context.Context, units []*component.Unit, opts *options.TerragruntOptions) error {
	for _, filter := range f.Filters {
		if err := filter.Filter(ctx, units, opts); err != nil {
			return err
		}
	}

	return nil
}

// UnitFilterGraph filters units to include only a target directory and its dependents.
// This filter is used by the graph command to show only relevant units in the dependency graph.
type UnitFilterGraph struct {
	// TargetDir is the directory whose dependents should be included
	TargetDir string
}

// Filter implements UnitFilter.
// It marks all units as excluded except for the target directory and units that depend on it.
func (f *UnitFilterGraph) Filter(ctx context.Context, units []*component.Unit, opts *options.TerragruntOptions) error {
	// Build dependency map first
	dependentUnits := make(map[string][]string)

	for _, unit := range units {
		for _, dep := range unit.Dependencies() {
			dependentUnits[dep.Path()] = util.RemoveDuplicatesFromList(
				append(dependentUnits[dep.Path()], unit.Path()),
			)
		}
	}

	// Propagate transitive dependencies across all units.
	// A DAG can have up to N−1 levels, so at most N iterations are needed.
	// Each iteration propagates one level deeper; exceeding N implies a cycle.
	//See: https://en.wikipedia.org/wiki/Topological_sorting#Properties
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
	modulesToInclude := dependentUnits[f.TargetDir]
	modulesToInclude = append(modulesToInclude, f.TargetDir)

	// Mark units as excluded unless they are in modulesToInclude
	for _, module := range units {
		excluded := true
		if util.ListContainsElement(modulesToInclude, module.Path()) {
			excluded = false
		}

		module.SetExcluded(excluded)

		if module.Execution != nil {
			module.Execution.FlagExcluded = excluded
		}
	}

	return nil
}

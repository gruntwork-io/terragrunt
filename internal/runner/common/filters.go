package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
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

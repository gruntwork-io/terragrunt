package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/telemetry"
)

// telemetryApplyFilters applies all configured unit filters to the resolved units
func (r *UnitResolver) telemetryApplyFilters(ctx context.Context, units Units) (Units, error) {
	if len(r.filters) == 0 {
		return units, nil
	}

	var filteredUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "apply_unit_filters", map[string]any{
		"working_dir":  r.Stack.TerragruntOptions.WorkingDir,
		"filter_count": len(r.filters),
	}, func(ctx context.Context) error {
		// Apply all filters in sequence
		for _, filter := range r.filters {
			if err := filter.Filter(ctx, units, r.Stack.TerragruntOptions); err != nil {
				return err
			}
		}

		filteredUnits = units

		return nil
	})

	return filteredUnits, err
}

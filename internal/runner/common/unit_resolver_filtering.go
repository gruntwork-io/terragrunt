package common

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
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

// telemetryApplyIncludeDirs applies include directory filters and sets FlagExcluded accordingly.
func (r *UnitResolver) telemetryApplyIncludeDirs(units Units) Units {
	if !r.Stack.TerragruntOptions.ExcludeByDefault {
		return units
	}

	for _, unit := range units {
		unit.FlagExcluded = true

		// Allow explicit include dirs when provided.
		for _, dir := range r.Stack.TerragruntOptions.IncludeDirs {
			if util.HasPathPrefix(unit.Path, dir) {
				unit.FlagExcluded = false
				break
			}
		}
	}

	return units
}

// telemetryFlagUnitsThatRead un-excludes units that read files in the Terragrunt configuration.
func (r *UnitResolver) telemetryFlagUnitsThatRead(units Units) Units {
	// Combine both UnitsReading (new) and ModulesThatInclude (legacy) for backwards compatibility
	filesToCheck := append(r.Stack.TerragruntOptions.ModulesThatInclude, r.Stack.TerragruntOptions.UnitsReading...) //nolint:gocritic

	if len(filesToCheck) == 0 {
		return units
	}

	normalizedPaths := []string{}

	for _, path := range filesToCheck {
		normalized := path

		if !filepath.IsAbs(normalized) {
			normalized = util.JoinPath(r.Stack.TerragruntOptions.WorkingDir, normalized)
		}

		normalized = util.CleanPath(normalized)

		normalizedPaths = append(normalizedPaths, normalized)
	}

	for _, normalizedPath := range normalizedPaths {
		for _, unit := range units {
			if slices.Contains(unit.Reading, normalizedPath) {
				unit.FlagExcluded = false
				continue
			}

			// Fallback: check ProcessedIncludes (include blocks from config)
			for _, includeConfig := range unit.Config.ProcessedIncludes {
				includePath := includeConfig.Path

				if !filepath.IsAbs(includePath) {
					includePath = util.JoinPath(r.Stack.TerragruntOptions.WorkingDir, includePath)
				}

				includePath = util.CleanPath(includePath)

				if includePath == normalizedPath {
					unit.FlagExcluded = false
					break
				}
			}
		}
	}

	return units
}

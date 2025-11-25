package common

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// telemetryApplyExcludeDirs re-applies exclude-dir reporting while preserving exclusion state.
func (r *UnitResolver) telemetryApplyExcludeDirs(l log.Logger, units Units) Units {
	matchesExclude := func(path string) bool {
		for _, dir := range r.Stack.TerragruntOptions.ExcludeDirs {
			cleanDir := dir
			if !filepath.IsAbs(cleanDir) {
				cleanDir = util.JoinPath(r.Stack.TerragruntOptions.WorkingDir, cleanDir)
			}

			cleanDir = util.CleanPath(cleanDir)

			if util.HasPathPrefix(path, cleanDir) {
				return true
			}
		}

		return false
	}

	reportUnitExclusion := func(unitPath string, reason report.Reason) {
		if r.Stack.Report == nil {
			return
		}

		absPath, err := EnsureAbsolutePath(unitPath)
		if err != nil {
			l.Errorf("Error getting absolute path for unit %s: %v", unitPath, err)
			return
		}

		absPath = util.CleanPath(absPath)

		run, err := r.Stack.Report.EnsureRun(absPath)
		if err != nil {
			l.Errorf("Error ensuring run for unit %s: %v", absPath, err)
			return
		}

		if err := r.Stack.Report.EndRun(
			run.Path,
			report.WithResult(report.ResultExcluded),
			report.WithReason(reason),
		); err != nil {
			l.Errorf("Error ending run for unit %s: %v", absPath, err)
			return
		}
	}

	for _, unit := range units {
		if matchesExclude(unit.Path) {
			unit.FlagExcluded = true

			reportUnitExclusion(unit.Path, report.ReasonExcludeDir)
		}

		for _, dep := range unit.Dependencies {
			if matchesExclude(dep.Path) {
				dep.FlagExcluded = true

				reportUnitExclusion(dep.Path, report.ReasonExcludeDir)
			}
		}
	}

	return units
}

// telemetryApplyExcludeModules records exclude-block exclusions to the report.
func (r *UnitResolver) telemetryApplyExcludeModules(l log.Logger, units Units) Units {
	matchesExclude := func(path string) bool {
		for _, dir := range r.Stack.TerragruntOptions.ExcludeDirs {
			cleanDir := dir
			if !filepath.IsAbs(cleanDir) {
				cleanDir = util.JoinPath(r.Stack.TerragruntOptions.WorkingDir, cleanDir)
			}

			cleanDir = util.CleanPath(cleanDir)

			if util.HasPathPrefix(path, cleanDir) {
				return true
			}
		}

		return false
	}

	reportUnitExclusion := func(unitPath string, reason report.Reason) {
		if r.Stack.Report == nil {
			return
		}

		absPath, err := EnsureAbsolutePath(unitPath)
		if err != nil {
			l.Errorf("Error getting absolute path for unit %s: %v", unitPath, err)
			return
		}

		absPath = util.CleanPath(absPath)

		run, err := r.Stack.Report.EnsureRun(absPath)
		if err != nil {
			l.Errorf("Error ensuring run for unit %s: %v", absPath, err)
			return
		}

		if err := r.Stack.Report.EndRun(
			run.Path,
			report.WithResult(report.ResultExcluded),
			report.WithReason(reason),
		); err != nil {
			l.Errorf("Error ending run for unit %s: %v", absPath, err)
			return
		}
	}

	for _, unit := range units {
		excludeConfig := unit.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(r.Stack.TerragruntOptions.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			wasAlreadyExcluded := unit.FlagExcluded
			unit.FlagExcluded = true

			// If CLI exclude-dir is set, prefer that reason for excluded units.
			if !wasAlreadyExcluded {
				reason := report.ReasonExcludeBlock
				if len(r.Stack.TerragruntOptions.ExcludeDirs) > 0 {
					reason = report.ReasonExcludeDir
				}

				if !matchesExclude(unit.Path) || reason == report.ReasonExcludeDir {
					reportUnitExclusion(unit.Path, reason)
				}
			}
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			for _, dependency := range unit.Dependencies {
				wasAlreadyExcluded := dependency.FlagExcluded
				dependency.FlagExcluded = true

				if !wasAlreadyExcluded {
					reason := report.ReasonExcludeBlock
					if len(r.Stack.TerragruntOptions.ExcludeDirs) > 0 {
						reason = report.ReasonExcludeDir
					}

					if !matchesExclude(dependency.Path) || reason == report.ReasonExcludeDir {
						reportUnitExclusion(dependency.Path, reason)
					}
				}
			}
		}
	}

	return units
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

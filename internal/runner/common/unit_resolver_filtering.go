package common

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gobwas/glob"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

// reportUnitExclusion records a unit exclusion in the report with proper error handling.
// Handles path normalization, duplicate prevention, and error logging.
func (r *UnitResolver) reportUnitExclusion(l log.Logger, unitPath string, reason report.Reason) {
	if r.Stack.Report == nil {
		return
	}

	// Ensure path is absolute for consistent reporting
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

// createPathMatcherFunc builds a matcher for include/exclude patterns.
// Why: centralizes path matching used by CLI flags and config.
// Matching: glob when doubleStarEnabled; otherwise exact path prefix.
// Mode: "include" uses include globs/dirs; "exclude" uses exclude globs/dirs.
// Examples: "**/staging/**", "modules/*/test", "envs/prod".
// Returns: func(*Unit) bool that is true when the unit matches.
func (r *UnitResolver) createPathMatcherFunc(mode string, opts *options.TerragruntOptions, l log.Logger) func(*Unit) bool {
	// Use glob matching when double-star is enabled, otherwise use exact path matching
	if r.doubleStarEnabled {
		var (
			globs  map[string]glob.Glob
			action string
		)

		if mode == "include" {
			globs = r.includeGlobs
			action = "included"
		} else {
			globs = r.excludeGlobs
			action = "excluded"
		}

		return func(unit *Unit) bool {
			for globPath, globPattern := range globs {
				if globPattern.Match(unit.Path()) {
					l.Debugf("Unit %s is %s by glob %s", unit.Path(), action, globPath)
					return true
				}
			}

			return false
		}
	}

	// Fallback to exact path matching when double-star is not enabled (backwards compatibility)
	var (
		dirs   []string
		action string
	)

	if mode == "include" {
		dirs = opts.IncludeDirs
		action = "included"
	} else {
		dirs = opts.ExcludeDirs
		action = "excluded"
	}

	return func(unit *Unit) bool {
		for _, dir := range dirs {
			if util.HasPathPrefix(unit.Path(), dir) {
				l.Debugf("Unit %s is %s by exact path match %s", unit.Path(), action, dir)
				return true
			}
		}

		return false
	}
}

// telemetryApplyFilters applies all configured unit filters to the resolved units
func (r *UnitResolver) telemetryApplyFilters(ctx context.Context, l log.Logger, units Units) (Units, error) {
	if len(r.filters) == 0 {
		return units, nil
	}

	var filteredUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "apply_unit_filters", map[string]any{
		"working_dir":  r.Stack.TerragruntOptions.WorkingDir,
		"filter_count": len(r.filters),
	}, func(ctx context.Context) error {
		// Track which units were excluded before filters were applied
		excludedBeforeFilters := make(map[string]bool)
		for _, unit := range units {
			excludedBeforeFilters[unit.Path()] = unit.Excluded()
		}

		// Apply all filters in sequence
		for _, filter := range r.filters {
			if err := filter.Filter(ctx, units, r.Stack.TerragruntOptions); err != nil {
				return err
			}
		}

		filteredUnits = units

		// Report exclusions for units that became excluded due to filters
		if r.Stack.Report != nil && l != nil {
			for _, unit := range units {
				// Only report if the unit is now excluded but wasn't excluded before filters
				if unit.Excluded() && !excludedBeforeFilters[unit.Path()] {
					r.reportUnitExclusion(l, unit.Path(), report.ReasonExcludeFilter)
				}
			}
		}

		return nil
	})

	return filteredUnits, err
}

// telemetryApplyIncludeDirs applies include directory filters and sets filterExcluded accordingly
func (r *UnitResolver) telemetryApplyIncludeDirs(ctx context.Context, l log.Logger, crossLinkedUnits Units) (Units, error) {
	var withUnitsIncluded Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "apply_include_dirs", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsIncluded = r.applyIncludeDirs(r.Stack.TerragruntOptions, l, crossLinkedUnits)
		return nil
	})

	return withUnitsIncluded, err
}

// applyIncludeDirs sets filterExcluded on units based on --queue-include-dir patterns (when ExcludeByDefault is true).
// Why: invert default behavior to run only requested units; optionally include deps unless StrictInclude.
// Matching: glob when doubleStarEnabled; otherwise exact path prefix.
// Behavior: no-op when ExcludeByDefault is false.
// When ExcludeByDefault is true but no include dirs are specified, excludes all units (used by --units-that-include).
// Examples: "**/prod/**", "apps/*/service-a", "envs/us-west-2".
func (r *UnitResolver) applyIncludeDirs(opts *options.TerragruntOptions, l log.Logger, units Units) Units {
	if !opts.ExcludeByDefault {
		return units
	}

	includeFn := r.createPathMatcherFunc("include", opts, l)

	for _, unit := range units {
		unit.SetFilterExcluded(true)

		if includeFn(unit) {
			unit.SetFilterExcluded(false)
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, unit := range units {
			if !unit.Excluded() {
				for _, dependency := range unit.Dependencies() {
					// Find the corresponding runner unit
					for _, u := range units {
						if u.Path() == dependency.Path() {
							u.SetFilterExcluded(false)
							break
						}
					}
				}
			}
		}
	}

	return units
}

// telemetryFlagUnitsThatRead flags units that read files in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagUnitsThatRead(ctx context.Context, withExcludedUnits Units) (Units, error) {
	var withUnitsRead Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_read", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsRead = r.flagUnitsThatRead(r.Stack.TerragruntOptions, withExcludedUnits)
		return nil
	})

	return withUnitsRead, err
}

// flagUnitsThatRead iterates over a unit slice and flags all units that read at least one file in the specified
// file list. This handles both --units-that-include (UnitsReading) and legacy ModulesThatInclude flags.
// Checks both unit.Reading (populated by discovery's FilesRead tracking) and Config.ProcessedIncludes (include blocks).
func (r *UnitResolver) flagUnitsThatRead(opts *options.TerragruntOptions, units Units) Units {
	// Combine both UnitsReading (new) and ModulesThatInclude (legacy) for backwards compatibility
	filesToCheck := append(opts.ModulesThatInclude, opts.UnitsReading...) //nolint:gocritic

	if len(filesToCheck) == 0 {
		return units
	}

	// Normalize paths to match the format used by config parsing.
	// Config joins relative paths with WorkingDir and cleans them.
	normalizedPaths := []string{}

	for _, path := range filesToCheck {
		normalized := path

		if !filepath.IsAbs(normalized) {
			normalized = util.JoinPath(opts.WorkingDir, normalized)
		}

		// Always clean the path (whether it was relative and joined, or already absolute)
		// to ensure consistent path separators across platforms (especially Windows)
		normalized = util.CleanPath(normalized)

		normalizedPaths = append(normalizedPaths, normalized)
	}

	// Check each unit against the normalized paths
	for _, normalizedPath := range normalizedPaths {
		for _, unit := range units {
			// Check unit.Reading (populated by discovery's FilesRead tracking)
			if slices.Contains(unit.Reading(), normalizedPath) {
				unit.SetFilterExcluded(false)
				continue
			}

			// Fallback: check Config.ProcessedIncludes (include blocks from config)
			// This is needed because unit.Reading may not be populated in all cases
			if unit.Config() != nil {
				for _, includeConfig := range unit.Config().ProcessedIncludes {
					if includeConfig.Path == normalizedPath {
						unit.SetFilterExcluded(false)
						break
					}
				}
			}
		}
	}

	return units
}

// telemetryApplyExcludeDirs applies exclude directory filters and sets filterExcluded accordingly
func (r *UnitResolver) telemetryApplyExcludeDirs(ctx context.Context, l log.Logger, withUnitsRead Units) (Units, error) {
	var withUnitsExcluded Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "apply_exclude_dirs", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsExcluded = r.applyExcludeDirs(l, r.Stack.TerragruntOptions, r.Stack.Report, withUnitsRead)
		return nil
	})

	return withUnitsExcluded, err
}

// applyExcludeDirs sets filterExcluded on units that match --queue-exclude-dir patterns.
// Why: enforce explicit user exclusions with highest precedence and preserve exclusion reasons in reports.
// Matching: uses glob patterns when doubleStarEnabled; otherwise exact path prefix matching.
// Examples:
//   - "**/staging/**"
//   - "modules/*/test"
//   - "envs/prod"
func (r *UnitResolver) applyExcludeDirs(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units Units) Units {
	// If we don't have any excludes, we don't need to do anything.
	if (len(r.excludeGlobs) == 0 && r.doubleStarEnabled) || len(opts.ExcludeDirs) == 0 {
		return units
	}

	excludeFn := r.createPathMatcherFunc("exclude", opts, l)

	for _, unit := range units {
		if excludeFn(unit) {
			// Mark unit itself as excluded
			unit.SetFilterExcluded(true)

			// Only update report if it's enabled
			if reportInstance != nil {
				r.reportUnitExclusion(l, unit.Path(), report.ReasonExcludeDir)
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range unit.Dependencies() {
			// Find the corresponding runner unit
			for _, u := range units {
				if u.Path() == dependency.Path() {
					if excludeFn(u) {
						u.SetFilterExcluded(true)

						// Only update report if it's enabled
						if reportInstance != nil {
							r.reportUnitExclusion(l, u.Path(), report.ReasonExcludeDir)
						}
					}

					break
				}
			}
		}
	}

	return units
}

// telemetryApplyExcludeModules applies exclude-modules filter and sets filterExcluded accordingly
func (r *UnitResolver) telemetryApplyExcludeModules(ctx context.Context, l log.Logger, withUnitsThatAreIncludedByOthers Units) (Units, error) {
	var withExcludedUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "apply_exclude_modules", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result := r.applyExcludeModules(l, r.Stack.TerragruntOptions, r.Stack.Report, withUnitsThatAreIncludedByOthers)
		withExcludedUnits = result

		return nil
	})

	return withExcludedUnits, err
}

// applyExcludeModules sets filterExcluded on units based on the exclude block in their terragrunt.hcl.
func (r *UnitResolver) applyExcludeModules(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units Units) Units {
	for _, unit := range units {
		cfg := unit.Config()
		if cfg == nil || cfg.Exclude == nil {
			continue
		}

		excludeConfig := cfg.Exclude

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			// Check if unit was already excluded (e.g., by --queue-exclude-dir)
			// If so, don't overwrite the existing exclusion reason
			wasAlreadyExcluded := unit.Excluded()
			unit.SetFilterExcluded(true)

			// Only update report if it's enabled AND the unit wasn't already excluded
			// This ensures CLI flags like --queue-exclude-dir take precedence over exclude blocks
			if reportInstance != nil && !wasAlreadyExcluded {
				r.reportUnitExclusion(l, unit.Path(), report.ReasonExcludeBlock)
			}
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for unit %s by exclude block", unit.Path())

			for _, dependency := range unit.Dependencies() {
				// Find the corresponding runner unit
				for _, u := range units {
					if u.Path() == dependency.Path() {
						// Check if dependency was already excluded
						wasAlreadyExcluded := u.Excluded()
						u.SetFilterExcluded(true)

						// Only update report if it's enabled AND the dependency wasn't already excluded
						// This ensures CLI exclusions take precedence over exclude blocks
						if reportInstance != nil && !wasAlreadyExcluded {
							r.reportUnitExclusion(l, u.Path(), report.ReasonExcludeBlock)
						}

						break
					}
				}
			}
		}
	}

	return units
}

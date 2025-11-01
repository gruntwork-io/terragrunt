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
	absPath := unitPath
	if !filepath.IsAbs(absPath) {
		p, err := filepath.Abs(unitPath)
		if err != nil {
			l.Errorf("Error getting absolute path for unit %s: %v", unitPath, err)
			return
		}

		absPath = p
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

// createPathMatcherFunc returns a function that checks if a unit matches configured patterns.
// Supports both glob patterns (when doubleStarEnabled) and exact path matching.
//
// Parameters:
//   - mode: "include" to match against includeGlobs/IncludeDirs, "exclude" for excludeGlobs/ExcludeDirs
//   - opts: TerragruntOptions containing the include/exclude dirs for exact matching mode
//   - l: Logger for debug output
//
// Returns a function that takes a *Unit and returns true if it matches the configured patterns.
func (r *UnitResolver) createPathMatcherFunc(mode string, opts *options.TerragruntOptions, l log.Logger) func(*Unit) bool {
	// Always use glob matching for pattern support
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
			if globPattern.Match(unit.Path) {
				l.Debugf("Unit %s is %s by glob %s", unit.Path, action, globPath)
				return true
			}
		}

		return false
	}
}

// telemetryFlagIncludedDirs flags directories that are included in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagIncludedDirs(ctx context.Context, l log.Logger, crossLinkedUnits Units) (Units, error) {
	var withUnitsIncluded Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_included_dirs", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsIncluded = r.flagIncludedDirs(r.Stack.TerragruntOptions, l, crossLinkedUnits)
		return nil
	})

	return withUnitsIncluded, err
}

// flagIncludedDirs applies include patterns when running in ExcludeByDefault mode.
//
// Behavior:
//   - When ExcludeByDefault is false: Returns units unchanged (all included by default)
//   - When ExcludeByDefault is true: Marks all units as excluded, then includes only those
//     matching the IncludeDirs patterns
//
// Include Mode:
//   - In StrictInclude mode: Only explicitly included units are processed
//   - In non-strict mode: Included units AND their dependencies are processed
//
// This is the 4th stage in the unit resolution pipeline.
//
// The ExcludeByDefault flag is set when using --terragrunt-include-dir, which inverts
// the normal inclusion logic: instead of including everything except excluded dirs,
// we exclude everything except included dirs.
func (r *UnitResolver) flagIncludedDirs(opts *options.TerragruntOptions, l log.Logger, units Units) Units {
	if !opts.ExcludeByDefault {
		return units
	}

	includeFn := r.createPathMatcherFunc("include", opts, l)

	for _, unit := range units {
		unit.FlagExcluded = true
		if includeFn(unit) {
			unit.FlagExcluded = false
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, unit := range units {
			if !unit.FlagExcluded {
				for _, dependency := range unit.Dependencies {
					dependency.FlagExcluded = false
				}
			}
		}
	}

	return units
}

// telemetryFlagUnitsThatAreIncluded flags units that are included in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagUnitsThatAreIncluded(ctx context.Context, withUnitsIncluded Units) (Units, error) {
	var withUnitsThatAreIncludedByOthers Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_units_that_are_included", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := r.flagUnitsThatAreIncluded(r.Stack.TerragruntOptions, withUnitsIncluded)
		if err != nil {
			return err
		}

		withUnitsThatAreIncludedByOthers = result

		return nil
	})

	return withUnitsThatAreIncludedByOthers, err
}

// flagUnitsThatAreIncluded marks units as included if they include specific configuration files.
//
// This is the 5th stage in the unit resolution pipeline. It handles the --terragrunt-modules-that-include
// flag, which selects units based on their included configuration files.
//
// The method:
//  1. Combines ModulesThatInclude and UnitsReading into a single list
//  2. Converts all paths to canonical form for reliable comparison
//  3. For each unit, checks if any of its ProcessedIncludes match the target files
//  4. For each unit's dependencies, checks their includes as well
//  5. Sets FlagExcluded=false for any unit or dependency that includes a target file
//
// This allows users to run commands on all units that include a specific configuration file,
// such as a common root.hcl or region.hcl file.
func (r *UnitResolver) flagUnitsThatAreIncluded(opts *options.TerragruntOptions, units Units) (Units, error) {
	unitsThatInclude := append(opts.ModulesThatInclude, opts.UnitsReading...) //nolint:gocritic

	if len(unitsThatInclude) == 0 {
		return units, nil
	}

	unitsThatIncludeCanonicalPaths := []string{}

	for _, includePath := range unitsThatInclude {
		canonicalPath, err := util.CanonicalPath(includePath, opts.WorkingDir)
		if err != nil {
			return nil, err
		}

		unitsThatIncludeCanonicalPaths = append(unitsThatIncludeCanonicalPaths, canonicalPath)
	}

	for _, unit := range units {
		if err := r.flagUnitIncludes(unit, unitsThatIncludeCanonicalPaths); err != nil {
			return nil, err
		}

		if err := r.flagUnitDependencies(unit, unitsThatIncludeCanonicalPaths); err != nil {
			return nil, err
		}
	}

	return units, nil
}

// flagUnitIncludes marks a unit as included if any of its include paths match the canonical paths.
// Returns an error if path resolution fails during the comparison.
func (r *UnitResolver) flagUnitIncludes(unit *Unit, canonicalPaths []string) error {
	for _, includeConfig := range unit.Config.ProcessedIncludes {
		canonicalPath, err := util.CanonicalPath(includeConfig.Path, unit.Path)
		if err != nil {
			return err
		}

		if util.ListContainsElement(canonicalPaths, canonicalPath) {
			unit.FlagExcluded = false
		}
	}

	return nil
}

// flagUnitDependencies processes dependencies of a unit and flags them based on include paths.
// Returns an error if dependency processing fails.
func (r *UnitResolver) flagUnitDependencies(unit *Unit, canonicalPaths []string) error {
	for _, dependency := range unit.Dependencies {
		if dependency.FlagExcluded {
			continue
		}

		if err := r.flagDependencyIncludes(dependency, unit.Path, canonicalPaths); err != nil {
			return err
		}
	}

	return nil
}

// flagDependencyIncludes marks a dependency as included if any of its include paths match the canonical paths.
// Returns an error if path resolution fails during the comparison.
func (r *UnitResolver) flagDependencyIncludes(dependency *Unit, unitPath string, canonicalPaths []string) error {
	for _, includeConfig := range dependency.Config.ProcessedIncludes {
		canonicalPath, err := util.CanonicalPath(includeConfig.Path, unitPath)
		if err != nil {
			return err
		}

		if util.ListContainsElement(canonicalPaths, canonicalPath) {
			dependency.FlagExcluded = false
		}
	}

	return nil
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
// file list in the TerragruntOptions UnitsReading attribute.
func (r *UnitResolver) flagUnitsThatRead(opts *options.TerragruntOptions, units Units) Units {
	// If no UnitsThatRead is specified, return the unit list instantly
	if len(opts.UnitsReading) == 0 {
		return units
	}

	for _, path := range opts.UnitsReading {
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.WorkingDir, path)
			path = filepath.Clean(path)
		}

		for _, unit := range units {
			if slices.Contains(unit.Reading, path) {
				unit.FlagExcluded = false
			}
		}
	}

	return units
}

// telemetryFlagExcludedDirs flags directories that are excluded in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagExcludedDirs(ctx context.Context, l log.Logger, withUnitsRead Units) (Units, error) {
	var withUnitsExcluded Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_dirs", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		withUnitsExcluded = r.flagExcludedDirs(l, r.Stack.TerragruntOptions, r.Stack.Report, withUnitsRead)
		return nil
	})

	return withUnitsExcluded, err
}

// flagExcludedDirs marks units as excluded if they match CLI exclude patterns.
//
// This is the 7th stage in the unit resolution pipeline. It applies the --terragrunt-exclude-dir
// flag to exclude units from execution.
//
// The method:
//  1. Checks if there are any exclude patterns to apply
//  2. For each unit, checks if it matches any exclude pattern
//  3. Marks matching units as excluded (FlagExcluded=true)
//  4. Also marks any matching dependencies as excluded
//  5. Reports exclusions with ReasonExcludeDir for tracking
//
// Pattern Matching:
//   - When doubleStarEnabled: Uses glob patterns (e.g., "**/staging/**")
//   - When disabled: Uses exact path matching
//
// Precedence:
//
//	This is the highest-precedence exclusion mechanism. Units excluded here will
//	have their exclusion reason preserved in reports, even if later stages would
//	also exclude them.
func (r *UnitResolver) flagExcludedDirs(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units Units) Units {
	// If we don't have any excludes, we don't need to do anything.
	if (len(r.excludeGlobs) == 0 && r.doubleStarEnabled) || len(opts.ExcludeDirs) == 0 {
		return units
	}

	excludeFn := r.createPathMatcherFunc("exclude", opts, l)

	for _, unit := range units {
		if excludeFn(unit) {
			// Mark unit itself as excluded
			unit.FlagExcluded = true

			// Only update report if it's enabled
			if reportInstance != nil {
				r.reportUnitExclusion(l, unit.Path, report.ReasonExcludeDir)
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range unit.Dependencies {
			if excludeFn(dependency) {
				dependency.FlagExcluded = true

				// Only update report if it's enabled
				if reportInstance != nil {
					r.reportUnitExclusion(l, dependency.Path, report.ReasonExcludeDir)
				}
			}
		}
	}

	return units
}

// telemetryFlagExcludedUnits flags units that are excluded in the Terragrunt configuration
func (r *UnitResolver) telemetryFlagExcludedUnits(ctx context.Context, l log.Logger, withUnitsThatAreIncludedByOthers Units) (Units, error) {
	var withExcludedUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "flag_excluded_units", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result := r.flagExcludedUnits(l, r.Stack.TerragruntOptions, r.Stack.Report, withUnitsThatAreIncludedByOthers)
		withExcludedUnits = result

		return nil
	})

	return withExcludedUnits, err
}

// flagExcludedUnits iterates over a unit slice and flags all units that are excluded based on the exclude block.
func (r *UnitResolver) flagExcludedUnits(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units Units) Units {
	for _, unit := range units {
		excludeConfig := unit.Config.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			// Check if unit was already excluded (e.g., by --queue-exclude-dir)
			// If so, don't overwrite the existing exclusion reason
			wasAlreadyExcluded := unit.FlagExcluded
			unit.FlagExcluded = true

			// Only update report if it's enabled AND the unit wasn't already excluded
			// This ensures CLI flags like --queue-exclude-dir take precedence over exclude blocks
			if reportInstance != nil && !wasAlreadyExcluded {
				r.reportUnitExclusion(l, unit.Path, report.ReasonExcludeBlock)
			}
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for unit %s by exclude block", unit.Path)

			for _, dependency := range unit.Dependencies {
				// Check if dependency was already excluded
				wasAlreadyExcluded := dependency.FlagExcluded
				dependency.FlagExcluded = true

				// Only update report if it's enabled AND the dependency wasn't already excluded
				// This ensures CLI exclusions take precedence over exclude blocks
				if reportInstance != nil && !wasAlreadyExcluded {
					r.reportUnitExclusion(l, dependency.Path, report.ReasonExcludeBlock)
				}
			}
		}
	}

	return units
}

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

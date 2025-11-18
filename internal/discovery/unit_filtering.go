package discovery

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/gobwas/glob"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

// reportUnitExclusion records a unit exclusion in the report with proper error handling.
// Handles path normalization, duplicate prevention, and error logging.
func (d *Discovery) reportUnitExclusion(l log.Logger, unitPath string, reason report.Reason) {
	if d.report == nil {
		return
	}

	// Ensure path is absolute for consistent reporting
	absPath, err := ensureAbsolutePath(unitPath)
	if err != nil {
		l.Errorf("Error getting absolute path for unit %s: %v", unitPath, err)
		return
	}

	absPath = util.CleanPath(absPath)

	run, err := d.report.EnsureRun(absPath)
	if err != nil {
		l.Errorf("Error ensuring run for unit %s: %v", absPath, err)
		return
	}

	if err := d.report.EndRun(
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
// Returns: func(*component.Unit) bool that is true when the unit matches.
func (d *Discovery) createPathMatcherFunc(mode string, opts *options.TerragruntOptions, l log.Logger) func(*component.Unit) bool {
	// Use glob matching when double-star is enabled, otherwise use exact path matching
	if d.doubleStarEnabled {
		var (
			globs  map[string]glob.Glob
			action string
		)

		if mode == "include" {
			globs = d.includeGlobs
			action = "included"
		} else {
			globs = d.excludeGlobs
			action = "excluded"
		}

		return func(unit *component.Unit) bool {
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

	return func(unit *component.Unit) bool {
		for _, dir := range dirs {
			if util.HasPathPrefix(unit.Path(), dir) {
				l.Debugf("Unit %s is %s by exact path match %s", unit.Path(), action, dir)
				return true
			}
		}

		return false
	}
}

// telemetryApplyIncludeDirs applies include directory filters and sets FlagExcluded accordingly
func (d *Discovery) telemetryApplyIncludeDirs(l log.Logger, crossLinkedUnits component.Units) component.Units {
	var withUnitsIncluded component.Units

	_ = telemetry.TelemeterFromContext(d.ctx).Collect(d.ctx, "apply_include_dirs", map[string]any{
		"working_dir": d.workingDir,
	}, func(_ context.Context) error {
		withUnitsIncluded = d.applyIncludeDirs(d.terragruntOptions, l, crossLinkedUnits)
		return nil
	})

	return withUnitsIncluded
}

// applyIncludeDirs sets FlagExcluded on units based on --queue-include-dir patterns (when ExcludeByDefault is true).
// Why: invert default behavior to run only requested units; optionally include deps unless StrictInclude.
// Matching: glob when doubleStarEnabled; otherwise exact path prefix.
// Behavior: no-op when ExcludeByDefault is false.
// When ExcludeByDefault is true but no include dirs are specified, excludes all units (used by --units-that-include).
// Examples: "**/prod/**", "apps/*/service-a", "envs/us-west-2".
func (d *Discovery) applyIncludeDirs(opts *options.TerragruntOptions, l log.Logger, units component.Units) component.Units {
	if !opts.ExcludeByDefault {
		return units
	}

	includeFn := d.createPathMatcherFunc("include", opts, l)

	for _, unit := range units {
		unit.SetFlagExcluded(true)

		if includeFn(unit) {
			unit.SetFlagExcluded(false)
		}
	}

	// Mark all affected dependencies as included before proceeding if not in strict include mode.
	if !opts.StrictInclude {
		for _, unit := range units {
			if !unit.FlagExcluded() {
				for _, dependency := range unit.Dependencies() {
					if dep, ok := dependency.(*component.Unit); ok {
						dep.SetFlagExcluded(false)
					}
				}
			}
		}
	}

	return units
}

// telemetryFlagUnitsThatRead flags units that read files in the Terragrunt configuration
func (d *Discovery) telemetryFlagUnitsThatRead(withExcludedUnits component.Units) component.Units {
	var withUnitsRead component.Units

	_ = telemetry.TelemeterFromContext(d.ctx).Collect(d.ctx, "flag_units_that_read", map[string]any{
		"working_dir": d.workingDir,
	}, func(_ context.Context) error {
		withUnitsRead = d.flagUnitsThatRead(d.terragruntOptions, withExcludedUnits)
		return nil
	})

	return withUnitsRead
}

// flagUnitsThatRead iterates over a unit slice and flags all units that read at least one file in the specified
// file list. This handles both --units-that-include (UnitsReading) and legacy ModulesThatInclude flags.
// Checks both unit.Reading (populated by discovery's FilesRead tracking) and Config.ProcessedIncludes (include blocks).
func (d *Discovery) flagUnitsThatRead(opts *options.TerragruntOptions, units component.Units) component.Units {
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
				unit.SetFlagExcluded(false)
				continue
			}

			// Fallback: check Config.ProcessedIncludes (include blocks from config)
			// This is needed because unit.Reading may not be populated in all cases
			cfg := unit.Config()
			if cfg != nil {
				for _, includeConfig := range cfg.ProcessedIncludes {
					if includeConfig.Path == normalizedPath {
						unit.SetFlagExcluded(false)
						break
					}
				}
			}
		}
	}

	return units
}

// telemetryApplyExcludeDirs applies exclude directory filters and sets FlagExcluded accordingly
func (d *Discovery) telemetryApplyExcludeDirs(l log.Logger, withUnitsRead component.Units) component.Units {
	var withUnitsExcluded component.Units

	_ = telemetry.TelemeterFromContext(d.ctx).Collect(d.ctx, "apply_exclude_dirs", map[string]any{
		"working_dir": d.workingDir,
	}, func(_ context.Context) error {
		withUnitsExcluded = d.applyExcludeDirs(l, d.terragruntOptions, d.report, withUnitsRead)
		return nil
	})

	return withUnitsExcluded
}

// applyExcludeDirs sets FlagExcluded on units that match --queue-exclude-dir patterns.
// Why: enforce explicit user exclusions with highest precedence and preserve exclusion reasons in reports.
// Matching: uses glob patterns when doubleStarEnabled; otherwise exact path prefix matching.
// Examples:
//   - "**/staging/**"
//   - "modules/*/test"
//   - "envs/prod"
func (d *Discovery) applyExcludeDirs(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units component.Units) component.Units {
	// If we don't have any excludes, we don't need to do anything.
	if (len(d.excludeGlobs) == 0 && d.doubleStarEnabled) || len(opts.ExcludeDirs) == 0 {
		return units
	}

	excludeFn := d.createPathMatcherFunc("exclude", opts, l)

	for _, unit := range units {
		if excludeFn(unit) {
			// Mark unit itself as excluded
			unit.SetFlagExcluded(true)

			// Only update report if it's enabled
			if reportInstance != nil {
				d.reportUnitExclusion(l, unit.Path(), report.ReasonExcludeDir)
			}
		}

		// Mark all affected dependencies as excluded
		for _, dependency := range unit.Dependencies() {
			if dep, ok := dependency.(*component.Unit); ok {
				if excludeFn(dep) {
					dep.SetFlagExcluded(true)

					// Only update report if it's enabled
					if reportInstance != nil {
						d.reportUnitExclusion(l, dep.Path(), report.ReasonExcludeDir)
					}
				}
			}
		}
	}

	return units
}

// telemetryApplyExcludeModules applies exclude-modules filter and sets FlagExcluded accordingly
func (d *Discovery) telemetryApplyExcludeModules(l log.Logger, withUnitsThatAreIncludedByOthers component.Units) component.Units {
	var withExcludedUnits component.Units

	_ = telemetry.TelemeterFromContext(d.ctx).Collect(d.ctx, "apply_exclude_modules", map[string]any{
		"working_dir": d.workingDir,
	}, func(_ context.Context) error {
		result := d.applyExcludeModules(l, d.terragruntOptions, d.report, withUnitsThatAreIncludedByOthers)
		withExcludedUnits = result

		return nil
	})

	return withExcludedUnits
}

// applyExcludeModules sets FlagExcluded on units based on the exclude block in their terragrunt.hcl.
func (d *Discovery) applyExcludeModules(l log.Logger, opts *options.TerragruntOptions, reportInstance *report.Report, units component.Units) component.Units {
	for _, unit := range units {
		cfg := unit.Config()
		if cfg == nil {
			continue
		}

		excludeConfig := cfg.Exclude

		if excludeConfig == nil {
			continue
		}

		if !excludeConfig.IsActionListed(opts.TerraformCommand) {
			continue
		}

		if excludeConfig.If {
			// Check if unit was already excluded (e.g., by --queue-exclude-dir)
			// If so, don't overwrite the existing exclusion reason
			wasAlreadyExcluded := unit.FlagExcluded()
			unit.SetFlagExcluded(true)

			// Only update report if it's enabled AND the unit wasn't already excluded
			// This ensures CLI flags like --queue-exclude-dir take precedence over exclude blocks
			if reportInstance != nil && !wasAlreadyExcluded {
				d.reportUnitExclusion(l, unit.Path(), report.ReasonExcludeBlock)
			}
		}

		if excludeConfig.ExcludeDependencies != nil && *excludeConfig.ExcludeDependencies {
			l.Debugf("Excluding dependencies for unit %s by exclude block", unit.Path())

			for _, dependency := range unit.Dependencies() {
				if dep, ok := dependency.(*component.Unit); ok {
					// Check if dependency was already excluded
					wasAlreadyExcluded := dep.FlagExcluded()
					dep.SetFlagExcluded(true)

					// Only update report if it's enabled AND the dependency wasn't already excluded
					// This ensures CLI exclusions take precedence over exclude blocks
					if reportInstance != nil && !wasAlreadyExcluded {
						d.reportUnitExclusion(l, dep.Path(), report.ReasonExcludeBlock)
					}
				}
			}
		}
	}

	return units
}

// ensureAbsolutePath ensures a path is absolute, converting it if necessary.
// Returns the absolute path and any error encountered during conversion.
func ensureAbsolutePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

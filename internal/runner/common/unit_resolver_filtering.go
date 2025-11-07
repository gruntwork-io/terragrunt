package common

import (
	"context"

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
				if globPattern.Match(unit.Component.Path()) {
					l.Debugf("Unit %s is %s by glob %s", unit.Component.Path(), action, globPath)
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
			if util.HasPathPrefix(unit.Component.Path(), dir) {
				l.Debugf("Unit %s is %s by exact path match %s", unit.Component.Path(), action, dir)
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
			excludedBeforeFilters[unit.Component.Path()] = unit.Component.Excluded()
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
				if unit.Component.Excluded() && !excludedBeforeFilters[unit.Component.Path()] {
					r.reportUnitExclusion(l, unit.Component.Path(), report.ReasonExcludeFilter)
				}
			}
		}

		return nil
	})

	return filteredUnits, err
}

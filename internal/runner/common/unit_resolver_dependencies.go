package common

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"
)

const maxLevelsOfRecursion = 20

// Look through the dependencies of the units in the given map and resolve the "external" dependency paths listed in
// each units config (i.e. those dependencies not in the given list of Terragrunt config canonical file paths).
// These external dependencies are outside of the current working directory, which means they may not be part of the
// environment the user is trying to run --all apply or run --all destroy. Therefore, this method also confirms whether the user wants
// to actually apply those dependencies or just assume they are already applied. Note that this method will NOT fill in
// the Dependencies field of the Unit struct (see the crosslinkDependencies method for that).
func (r *UnitResolver) resolveExternalDependenciesForUnits(ctx context.Context, l log.Logger, unitsMap, unitsAlreadyProcessed UnitsMap, recursionLevel int) (UnitsMap, error) {
	allExternalDependencies := UnitsMap{}
	unitsToSkip := unitsMap.MergeMaps(unitsAlreadyProcessed)

	// Simple protection from circular dependencies causing a Stack Overflow due to infinite recursion
	if recursionLevel > maxLevelsOfRecursion {
		return allExternalDependencies, errors.New(InfiniteRecursionError{RecursionLevel: maxLevelsOfRecursion, Units: unitsToSkip})
	}

	sortedKeys := unitsMap.SortedKeys()
	for _, key := range sortedKeys {
		unit := unitsMap[key]

		// Check if this unit has dependencies that are considered "external"
		if unit.Config.Dependencies == nil || len(unit.Config.Dependencies.Paths) == 0 {
			continue
		}

		l, unitOpts, err := r.Stack.TerragruntOptions.CloneWithConfigPath(l, config.GetDefaultConfigPath(unit.Path))
		if err != nil {
			return nil, err
		}

		// For each dependency, check if it's external (outside working dir) and already in unitsToSkip
		for _, dependencyPath := range unit.Config.Dependencies.Paths {
			canonicalPath, err := util.CanonicalPath(dependencyPath, unit.Path)
			if err != nil {
				return nil, err
			}

			// Get the dependency unit from unitsToSkip first (it should be there from discovery)
			externalDependency, found := unitsToSkip[canonicalPath]
			if !found {
				l.Debugf("Dependency %s of unit %s not found in unitsMap (may be excluded or outside discovery scope)", canonicalPath, unit.Path)
				continue
			}

			// Skip if not external (inside working directory)
			// Convert both paths to absolute for proper comparison
			absCanonicalPath, err := filepath.Abs(canonicalPath)
			if err != nil {
				return nil, err
			}

			absWorkingDir, err := filepath.Abs(r.Stack.TerragruntOptions.WorkingDir)
			if err != nil {
				return nil, err
			}

			if util.HasPathPrefix(absCanonicalPath, absWorkingDir) {
				l.Debugf("Dependency %s is inside working directory, not treating as external", canonicalPath)
				continue
			}

			l.Debugf("Dependency %s is outside working directory, treating as external", canonicalPath)

			// Skip if already processed
			if _, alreadyFound := allExternalDependencies[externalDependency.Path]; alreadyFound {
				continue
			}

			shouldApply := false
			if !r.Stack.TerragruntOptions.IgnoreExternalDependencies {
				shouldApply, err = r.confirmShouldApplyExternalDependency(ctx, unit, l, externalDependency, unitOpts)
				if err != nil {
					return nil, err
				}
			}

			externalDependency.AssumeAlreadyApplied = !shouldApply
			// Mark external dependencies as excluded if they shouldn't be applied
			// This ensures they are tracked in the report but not executed
			if !shouldApply {
				externalDependency.FlagExcluded = true
			}

			allExternalDependencies[externalDependency.Path] = externalDependency
		}
	}

	if len(allExternalDependencies) > 0 {
		recursiveDependencies, err := r.resolveExternalDependenciesForUnits(ctx, l, allExternalDependencies, unitsMap, recursionLevel+1)
		if err != nil {
			return allExternalDependencies, err
		}

		return allExternalDependencies.MergeMaps(recursiveDependencies), nil
	}

	return allExternalDependencies, nil
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given unit is already
// applied. If the user selects "yes", then Terragrunt will apply that unit as well.
// Note that we skip the prompt for `run --all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --queue-include-external or --queue-include-dir flags.
func (r *UnitResolver) confirmShouldApplyExternalDependency(ctx context.Context, unit *Unit, l log.Logger, dependency *Unit, opts *options.TerragruntOptions) (bool, error) {
	if opts.IncludeExternalDependencies {
		l.Debugf("The --queue-include-external flag is set, so automatically including all external dependencies, and will run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return true, nil
	}

	if opts.NonInteractive {
		l.Debugf("The --non-interactive flag is set. To avoid accidentally affecting external dependencies with a run --all command, will not run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return false, nil
	}

	stackCmd := opts.TerraformCommand
	if stackCmd == "destroy" {
		l.Debugf("run --all command called with destroy. To avoid accidentally having destructive effects on external dependencies with run --all command, will not run this command against unit %s, which is a dependency of unit %s.", dependency.Path, unit.Path)
		return false, nil
	}

	l.Infof("Unit %s has external dependency %s", unit.Path, dependency.Path)

	return shell.PromptUserForYesNo(ctx, l, "Should Terragrunt apply the external dependency?", opts)
}

// telemetryCrossLinkDependencies cross-links dependencies between units
func (r *UnitResolver) telemetryCrossLinkDependencies(ctx context.Context, unitsMap, externalDependencies UnitsMap, canonicalTerragruntConfigPaths []string) (Units, error) {
	var crossLinkedUnits Units

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "crosslink_dependencies", map[string]any{
		"working_dir": r.Stack.TerragruntOptions.WorkingDir,
	}, func(_ context.Context) error {
		result, err := unitsMap.MergeMaps(externalDependencies).CrossLinkDependencies(canonicalTerragruntConfigPaths)
		if err != nil {
			return err
		}

		crossLinkedUnits = result

		return nil
	})

	return crossLinkedUnits, err
}

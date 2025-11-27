package common

import (
	"context"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/shell"
)

// flagExternalDependencies processes units that were marked as external by discovery,
// prompting the user whether to install them and setting appropriate flags.
// Discovery has already found, parsed, and marked external dependencies.
// This function only handles the user-facing logic for deciding whether to run them.
func (r *UnitResolver) flagExternalDependencies(ctx context.Context, l log.Logger, unitsMap UnitsMap) error {
	for _, unit := range unitsMap {
		// Check if this unit was marked as external by discovery
		// External units are outside the working directory
		if !unit.IsExternal {
			continue
		}

		shouldApply := false

		if !r.Stack.TerragruntOptions.IgnoreExternalDependencies {
			// Find a unit that depends on this external dependency for context
			var dependentUnit *Unit

			for _, u := range unitsMap {
				for _, dep := range u.Dependencies {
					if dep.Path == unit.Path {
						dependentUnit = u
						break
					}
				}

				if dependentUnit != nil {
					break
				}
			}

			// If we found a dependent, ask the user
			if dependentUnit != nil {
				var err error

				shouldApply, err = r.confirmShouldInstallExternalDependency(ctx, dependentUnit, l, unit, unit.TerragruntOptions)
				if err != nil {
					return err
				}
			}
		}

		unit.AssumeAlreadyApplied = !shouldApply
		// Mark external dependencies as excluded if they shouldn't be applied
		// This ensures they are tracked in the report but not executed
		if !shouldApply {
			unit.FlagExcluded = true
		}
	}

	return nil
}

// Confirm with the user whether they want Terragrunt to assume the given dependency of the given unit is already
// applied. If the user selects "yes", then Terragrunt will install that unit as well.
// Note that we skip the prompt for `run --all destroy` calls. Given the destructive and irreversible nature of destroy, we don't
// want to provide any risk to the user of accidentally destroying an external dependency unless explicitly included
// with the --queue-include-external or --queue-include-dir flags.
func (r *UnitResolver) confirmShouldInstallExternalDependency(ctx context.Context, unit *Unit, l log.Logger, dependency *Unit, opts *options.TerragruntOptions) (bool, error) {
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

	return shell.PromptUserForYesNo(ctx, l, "Should Terragrunt install the external dependency?", opts)
}

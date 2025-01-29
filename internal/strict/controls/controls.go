// Package controls contains strict controls.
package controls

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
)

const (
	LegacyRunAll = "run-all-commands"

	LegacyLogs = "legacy-logs"

	CLIRedesign = "cli-redesign"

	// DeprecatedCommands is the control that prevents the use of deprecated commands.
	DeprecatedCommands = "deprecated-commands"

	// DeprecatedFlags is the control that prevents the use of deprecated flag names.
	DeprecatedFlags = "deprecated-flags"

	// DeprecatedEnvVars is the control that prevents the use of deprecated env vars.
	DeprecatedEnvVars = "deprecated-env-vars"

	// RootTerragruntHCL is the control that prevents usage of a `terragrunt.hcl` file as the root of Terragrunt configurations.
	RootTerragruntHCL = "root-terragrunt-hcl"

	// SkipDependenciesInputs is the control that prevents reading dependencies inputs and get performance boost.
	SkipDependenciesInputs = "skip-dependencies-inputs"
)

//nolint:lll
func New() strict.Controls {
	lifetimeCategory := &strict.Category{
		Name:            "Lifetime controls",
		AllowedStatuses: strict.Statuses{strict.ActiveStatus},
	}
	stageCategory := &strict.Category{
		Name:            "Stage controls",
		ShowStatus:      true,
		AllowedStatuses: strict.Statuses{strict.ActiveStatus, strict.CompletedStatus},
	}

	controls := strict.Controls{
		&Control{
			Name:        DeprecatedCommands,
			Description: "Prevents deprecated commands from being used.",
			Category:    lifetimeCategory,
		},
		&Control{
			Name:        DeprecatedFlags,
			Description: "Prevents deprecated flags from being used.",
			Category:    lifetimeCategory,
		},
		&Control{
			Name:        DeprecatedEnvVars,
			Description: "Prevents deprecated env vars from being used.",
			Category:    lifetimeCategory,
		},
		&Control{
			Name:        LegacyRunAll,
			Description: "Prevents old *-all commands such as plan-all from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        CLIRedesign,
			Description: "Prevents old design CLI flags/commands from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        LegacyLogs,
			Description: "Prevents old log flags from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        RootTerragruntHCL,
			Description: "Throw an error when users try to reference a root terragrunt.hcl file using find_in_parent_folders.",
			Error:       errors.New("Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer supported. Use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl"),
			Warning:     "Using `terragrunt.hcl` as the root of Terragrunt configurations is an anti-pattern, and no longer recommended. In a future version of Terragrunt, this will result in an error. You are advised to use a differently named file like `root.hcl` instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/migrating-from-root-terragrunt-hcl",
			Category:    stageCategory,
		},
		&Control{
			// TODO: `ErrorFmt` and `WarnFmt` of this control are not displayed anywhere and needs to be reworked.
			Name:        SkipDependenciesInputs,
			Description: "Disable reading of dependency inputs to enhance dependency resolution performance by preventing recursively parsing Terragrunt inputs from dependencies.",
			Error:       errors.Errorf("The `%s` option is deprecated. Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. To continue using inputs from dependencies, forward them as outputs.", SkipDependenciesInputs),
			Warning:     fmt.Sprintf("The `%s` option is deprecated and will be removed in a future version of Terragrunt. Reading inputs from dependencies has been deprecated. To continue using inputs from dependencies, forward them as outputs.", SkipDependenciesInputs),
			Category:    stageCategory,
		},
	}

	return append(controls, NewSuspendedControls()...).Sort()
}

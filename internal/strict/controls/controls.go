// Package controls contains strict controls.
package controls

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict"
)

const (
	// DeprecatedCommands is the control that prevents the use of deprecated commands.
	DeprecatedCommands = "deprecated-commands"

	// DeprecatedFlags is the control that prevents the use of deprecated flag names.
	DeprecatedFlags = "deprecated-flags"

	// DeprecatedEnvVars is the control that prevents the use of deprecated env vars.
	DeprecatedEnvVars = "deprecated-env-vars"

	// DeprecatedConfigs is the control that prevents the use of deprecated config fields/section/..., anything related to config syntax.
	DeprecatedConfigs = "deprecated-configs"

	// LegacyAll is a control group for the legacy *-all commands.
	LegacyAll = "legacy-all"

	// LegacyLogs is a control group for legacy log flags that were in use before the log was redesign.
	LegacyLogs = "legacy-logs"

	// TerragruntPrefixFlags is a control group for flags that used to have the `terragrunt-` prefix.
	TerragruntPrefixFlags = "terragrunt-prefix-flags"

	// TerragruntPrefixEnvVars is a control group for env vars that used to have the `TERRAGRUNT_` prefix.
	TerragruntPrefixEnvVars = "terragrunt-prefix-env-vars"

	// DefaultCommands is a control group for TF commands that were used as default commands,
	// namely without using the parent `run` commands and were not shortcuts commands.
	DefaultCommands = "default-commands"

	// RootTerragruntHCL is the control that prevents usage of a `terragrunt.hcl` file as the root of Terragrunt configurations.
	RootTerragruntHCL = "root-terragrunt-hcl"

	// SkipDependenciesInputs is the control that prevents reading dependencies inputs and get performance boost.
	SkipDependenciesInputs = "skip-dependencies-inputs"
)

//nolint:lll
func New() strict.Controls {
	lifecycleCategory := &strict.Category{
		Name: "Lifecycle controls",
	}
	stageCategory := &strict.Category{
		Name: "Stage controls",
	}

	skipDependenciesInputsControl := &Control{
		// TODO: `ErrorFmt` and `WarnFmt` of this control are not displayed anywhere and needs to be reworked.
		Name:        SkipDependenciesInputs,
		Description: "Disable reading of dependency inputs to enhance dependency resolution performance by preventing recursively parsing Terragrunt inputs from dependencies.",
		Error:       errors.Errorf("Reading inputs from dependencies is no longer supported. To acquire values from dependencies, use outputs."),
		Warning:     "Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. If a value in a dependency is needed, use dependency outputs instead.",
		Category:    stageCategory,
	}

	controls := strict.Controls{
		&Control{
			Name:        DeprecatedCommands,
			Description: "Prevents deprecated commands from being used.",
			Category:    lifecycleCategory,
		},
		&Control{
			Name:        DeprecatedFlags,
			Description: "Prevents deprecated flags from being used.",
			Category:    lifecycleCategory,
		},
		&Control{
			Name:        DeprecatedEnvVars,
			Description: "Prevents deprecated env vars from being used.",
			Category:    lifecycleCategory,
		},
		&Control{
			Name:        DeprecatedConfigs,
			Description: "Prevents deprecated config syntax from being used.",
			Category:    lifecycleCategory,
			Subcontrols: strict.Controls{
				skipDependenciesInputsControl,
			},
		},
		skipDependenciesInputsControl,
		&Control{
			Name:        LegacyAll,
			Description: "Prevents old *-all commands such as plan-all from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        TerragruntPrefixFlags,
			Description: "Prevents deprecated flags with `terragrunt-` prefixes from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        TerragruntPrefixEnvVars,
			Description: "Prevents deprecated env vars with `TERRAGRUNT_` prefixes from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        DefaultCommands,
			Description: "Prevents default commands from being used.",
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
			Name:        "spin-up",
			Description: "Prevents the deprecated spin-up command from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        "tear-down",
			Description: "Prevents the deprecated tear-down command from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        "plan-all",
			Description: "Prevents the deprecated plan-all command from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        "apply-all",
			Description: "Prevents the deprecated apply-all command from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        "destroy-all",
			Description: "Prevents the deprecated destroy-all command from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        "output-all",
			Description: "Prevents the deprecated output-all command from being used.",
			Category:    stageCategory,
		},
		&Control{
			Name:        "validate-all",
			Description: "Prevents the deprecated validate-all command from being used.",
			Category:    stageCategory,
		},
	}

	return controls.Sort()
}

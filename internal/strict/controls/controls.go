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

	// SkipDependenciesInputs is the control related to the deprecated dependency inputs feature.
	// Dependency inputs are now disabled by default for performance.
	SkipDependenciesInputs = "skip-dependencies-inputs"

	// RequireExplicitBootstrap is the control that prevents the backend for remote state from being bootstrapped unless the `--backend-bootstrap` flag is specified.
	RequireExplicitBootstrap = "require-explicit-bootstrap"

	// CLIRedesign is the control that prevents the use of commands deprecated as part of the CLI Redesign.
	CLIRedesign = "cli-redesign"

	// LegacyAll is a control group for the legacy *-all commands.
	// This control is marked as completed since the commands have been removed.
	LegacyAll = "legacy-all"

	// BareInclude is the control that prevents the use of the `include` block without a label.
	BareInclude = "bare-include"

	// DoubleStar enables the use of the `**` glob pattern as a way to match files in subdirectories.
	// and will log a warning when using **/*
	DoubleStar = "double-star"

	// QueueExcludeExternal is the control that prevents the use of the deprecated `--queue-exclude-external` flag.
	QueueExcludeExternal = "queue-exclude-external"
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
		Name:        SkipDependenciesInputs,
		Description: "Controls whether to allow the deprecated dependency inputs feature. Dependency inputs are now disabled by default for performance. Use dependency outputs instead.",
		Error:       errors.Errorf("Reading inputs from dependencies is no longer supported. To acquire values from dependencies, use outputs."),
		Warning:     "Reading inputs from dependencies has been deprecated and is now disabled by default for performance. Use dependency outputs instead.",
		Category:    stageCategory,
		Status:      strict.CompletedStatus,
	}

	requireExplicitBootstrapControl := &Control{
		Name:        RequireExplicitBootstrap,
		Description: "Don't bootstrap backends by default. When enabled, users must supply `--backend-bootstrap` explicitly to automatically bootstrap backend resources.",
		Error:       errors.Errorf("Bootstrap backend for remote state by default is no longer supported. Use `--backend-bootstrap` flag instead."),
		Warning:     "Bootstrapping backend resources by default is deprecated functionality, and will not be the default behavior in a future version of Terragrunt. Use the explicit `--backend-bootstrap` flag to automatically provision backend resources before they're needed.",
		Category:    stageCategory,
		Status:      strict.CompletedStatus,
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
				requireExplicitBootstrapControl,
			},
		},
		skipDependenciesInputsControl,
		requireExplicitBootstrapControl,
		&Control{
			Name:        CLIRedesign,
			Description: "Prevents the use of commands deprecated as part of the CLI Redesign.",
			Category:    stageCategory,
		},
		&Control{
			Name:        LegacyAll,
			Description: "Prevents old *-all commands such as plan-all from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "spin-up",
			Description: "Prevents the deprecated spin-up command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "tear-down",
			Description: "Prevents the deprecated tear-down command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "plan-all",
			Description: "Prevents the deprecated plan-all command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "apply-all",
			Description: "Prevents the deprecated apply-all command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "destroy-all",
			Description: "Prevents the deprecated destroy-all command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "output-all",
			Description: "Prevents the deprecated output-all command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},
		&Control{
			Name:        "validate-all",
			Description: "Prevents the deprecated validate-all command from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
		},

		&Control{
			Name:        TerragruntPrefixFlags,
			Description: "Prevents deprecated flags with `terragrunt-` prefixes from being used.",
			Category:    stageCategory,
			Status:      strict.CompletedStatus,
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
			Name:        BareInclude,
			Description: "Prevents the use of the `include` block without a label.",
			Category:    stageCategory,
			Error:       errors.New("Using an `include` block without a label is deprecated. Please use the `include` block with a label instead."),
			Warning:     "Using an `include` block without a label is deprecated. Please use the `include` block with a label instead. For more information, see https://terragrunt.gruntwork.io/docs/migrate/bare-include/",
		},

		&Control{
			Name:        DoubleStar,
			Description: "Use the `**` glob pattern to select all files in a directory and its subdirectories.",
			Category:    stageCategory,
			Error:       errors.New("Using `**` to select all files in a directory and its subdirectories is enabled. **/* now matches subdirectories with at least a depth of one."),
			Warning:     "Using `**` to select all files in a directory and its subdirectories is enabled. **/* now matches subdirectories with at least a depth of one.",
		},
		&Control{
			Name:        QueueExcludeExternal,
			Description: "Prevents the use of the deprecated `--queue-exclude-external` flag. External dependencies are now excluded by default.",
			Category:    stageCategory,
			Error:       errors.New("The `--queue-exclude-external` flag is no longer supported. External dependencies are now excluded by default. Use --queue-include-external to include them."),
			Warning:     "The `--queue-exclude-external` flag is deprecated and will be removed in a future version of Terragrunt. External dependencies are now excluded by default.",
		},
	}

	return controls.Sort()
}

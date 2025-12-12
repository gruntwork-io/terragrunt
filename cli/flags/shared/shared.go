// Package shared provides flags that are shared by multiple commands.
//
// This package is underutilized right now, as some more serious refactoring is needed to make sure all
// shared flags use this package instead of reusing flags from other commands.
package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// General related flags.
	TFPathFlagName = "tf-path"

	// Queue related flags.
	QueueIgnoreErrorsFlagName        = "queue-ignore-errors"
	QueueIgnoreDAGOrderFlagName      = "queue-ignore-dag-order"
	QueueExcludeExternalFlagName     = "queue-exclude-external"
	QueueExcludeDirFlagName          = "queue-exclude-dir"
	QueueExcludesFileFlagName        = "queue-excludes-file"
	QueueIncludeDirFlagName          = "queue-include-dir"
	QueueIncludeExternalFlagName     = "queue-include-external"
	QueueStrictIncludeFlagName       = "queue-strict-include"
	QueueIncludeUnitsReadingFlagName = "queue-include-units-reading"

	// Filter related flags.
	FilterFlagName             = "filter"
	FilterAllowDestroyFlagName = "filter-allow-destroy"

	// Scaffolding related flags.
	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"
	NoShellFlagName       = "no-shell"
	NoHooksFlagName       = "no-hooks"

	// Concurrency control flags.
	ParallelismFlagName = "parallelism"

	// Backend flags
	BackendBootstrapFlagName        = "backend-bootstrap"
	BackendRequireBootstrapFlagName = "backend-require-bootstrap"
	DisableBucketUpdateFlagName     = "disable-bucket-update"

	// Feature flags
	FeatureFlagName = "feature"

	// Config and download flags
	ConfigFlagName      = "config"
	DownloadDirFlagName = "download-dir"

	// Auth and IAM flags
	AuthProviderCmdFlagName               = "auth-provider-cmd"
	InputsDebugFlagName                   = "inputs-debug"
	IAMAssumeRoleFlagName                 = "iam-assume-role"
	IAMAssumeRoleDurationFlagName         = "iam-assume-role-duration"
	IAMAssumeRoleSessionNameFlagName      = "iam-assume-role-session-name"
	IAMAssumeRoleWebIdentityTokenFlagName = "iam-assume-role-web-identity-token"
)

// NewTFPathFlag creates a flag for specifying the OpenTofu/Terraform binary path.
func NewTFPathFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(
		&cli.GenericFlag[string]{
			Name:    TFPathFlagName,
			EnvVars: tgPrefix.EnvVars(TFPathFlagName),
			Usage:   "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
			Setter: func(value string) error {
				opts.TFPath = value
				opts.TFPathExplicitlySet = true
				return nil
			},
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("tfpath"), terragruntPrefixControl),
	)
}

// NewQueueFlags creates the flags used for queue control
func NewQueueFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return cli.Flags{
		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueIgnoreErrorsFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreErrorsFlagName),
				Destination: &opts.IgnoreDependencyErrors,
				Usage:       "Continue processing Units even if a dependency fails.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-dependency-errors"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueIgnoreDAGOrderFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIgnoreDAGOrderFlagName),
				Destination: &opts.IgnoreDependencyOrder,
				Usage:       "Ignore DAG order for --all commands.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-dependency-order"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueExcludeExternalFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludeExternalFlagName),
				Destination: &opts.IgnoreExternalDependencies,
				Usage:       "Ignore external dependencies for --all commands.",
				Hidden:      true,
				Action: func(ctx *cli.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.QueueExcludeExternal).Evaluate(ctx.Context)
					}
					return nil
				},
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("ignore-external-dependencies"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueIncludeExternalFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIncludeExternalFlagName),
				Destination: &opts.IncludeExternalDependencies,
				Usage:       "Include external dependencies for --all commands without asking.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-external-dependencies"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        QueueExcludesFileFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludesFileFlagName),
				Destination: &opts.ExcludesFile,
				Usage:       "Path to a file with a list of directories that need to be excluded when running *-all commands.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("excludes-file"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        QueueExcludeDirFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueExcludeDirFlagName),
				Destination: &opts.ExcludeDirs,
				Usage:       "Unix-style glob of directories to exclude from the queue of Units to run.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("exclude-dir"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        QueueIncludeDirFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIncludeDirFlagName),
				Destination: &opts.IncludeDirs,
				Usage:       "Unix-style glob of directories to include from the queue of Units to run.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("include-dir"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        QueueStrictIncludeFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueStrictIncludeFlagName),
				Destination: &opts.StrictInclude,
				Usage:       "If flag is set, only modules under the directories passed in with '--queue-include-dir' will be included.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("strict-include"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        QueueIncludeUnitsReadingFlagName,
				EnvVars:     tgPrefix.EnvVars(QueueIncludeUnitsReadingFlagName),
				Destination: &opts.UnitsReading,
				Usage:       "If flag is set, 'run --all' will only run the command against units that read the specified file via a Terragrunt HCL function or include.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("queue-include-units-reading"), terragruntPrefixControl),
		),
	}
}

// NewFilterFlags creates flags for filter functionality.
func NewFilterFlags(opts *options.TerragruntOptions) cli.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return cli.Flags{
		flags.NewFlag(
			&cli.SliceFlag[string]{
				Name:        FilterFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterFlagName),
				Destination: &opts.FilterQueries,
				Usage:       "Filter components using filter syntax. Can be specified multiple times for union (OR) semantics. Requires the 'filter' experiment.",
				Action: func(_ *cli.Context, val []string) error {
					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --filter flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}
					return nil
				},
			},
		),
		flags.NewFlag(
			&cli.BoolFlag{
				Name:        FilterAllowDestroyFlagName,
				EnvVars:     tgPrefix.EnvVars(FilterAllowDestroyFlagName),
				Destination: &opts.FilterAllowDestroy,
				Usage:       "Allow destroy runs when using Git-based filters. Requires the 'filter-flag' experiment.",
				Action: func(_ *cli.Context, val bool) error {
					// Check if the filter-flag experiment is enabled
					if !opts.Experiments.Evaluate("filter-flag") {
						return cli.NewExitError("the --filter-allow-destroy flag requires the 'filter-flag' experiment to be enabled. Use --experiment=filter-flag or --experiment-mode to enable it", cli.ExitCodeGeneralError)
					}
					return nil
				},
			},
		),
	}
}

// NewScaffoldingFlags creates the flags shared between catalog and scaffold commands.
func NewScaffoldingFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        RootFileNameFlagName,
			EnvVars:     tgPrefix.EnvVars(RootFileNameFlagName),
			Destination: &opts.ScaffoldRootFileName,
			Usage:       "Name of the root Terragrunt configuration file, if used.",
			Action: func(ctx *cli.Context, value string) error {
				if value == "" {
					return cli.NewExitError("root-file-name flag cannot be empty", cli.ExitCodeGeneralError)
				}

				if value != opts.TerragruntConfigPath {
					opts.ScaffoldRootFileName = value

					return nil
				}

				if err := opts.StrictControls.FilterByNames("RootTerragruntHCL").Evaluate(ctx); err != nil {
					return cli.NewExitError(err, cli.ExitCodeGeneralError)
				}

				return nil
			},
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoIncludeRootFlagName,
			EnvVars:     tgPrefix.EnvVars(NoIncludeRootFlagName),
			Destination: &opts.ScaffoldNoIncludeRoot,
			Usage:       "Do not include root unit in scaffolding done by catalog.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoShellFlagName,
			EnvVars:     tgPrefix.EnvVars(NoShellFlagName),
			Destination: &opts.NoShell,
			Usage:       "Disable shell commands when using boilerplate templates.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        NoHooksFlagName,
			EnvVars:     tgPrefix.EnvVars(NoHooksFlagName),
			Destination: &opts.NoHooks,
			Usage:       "Disable hooks when using boilerplate templates.",
		}),
	}
}

// NewParallelismFlag creates a flag for specifying parallelism level.
func NewParallelismFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(
		&cli.GenericFlag[int]{
			Name:        ParallelismFlagName,
			EnvVars:     tgPrefix.EnvVars(ParallelismFlagName),
			Destination: &opts.Parallelism,
			Usage:       "Parallelism for --all commands.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("parallelism"), terragruntPrefixControl),
	)
}

// NewBackendFlags defines backend-related flags that should be available to both `run` and `backend` commands.
func NewBackendFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        BackendBootstrapFlagName,
			EnvVars:     tgPrefix.EnvVars(BackendBootstrapFlagName),
			Destination: &opts.BackendBootstrap,
			Usage:       "Automatically bootstrap backend infrastructure before attempting to use it.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     tgPrefix.EnvVars(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("fail-on-state-bucket-creation"), terragruntPrefixControl),
		),
		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("disable-bucket-update"), terragruntPrefixControl),
		),
	}
}

// NewFeatureFlags defines the feature flag map that should be available to both `run` and `backend` commands.
func NewFeatureFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return cli.Flags{
		flags.NewFlag(&cli.MapFlag[string, string]{
			Name:    FeatureFlagName,
			EnvVars: tgPrefix.EnvVars(FeatureFlagName),
			Usage:   "Set feature flags for the HCL code.",
			// Use default splitting behavior with comma separators via MapFlag defaults
			Action: func(_ *cli.Context, value map[string]string) error {
				for key, val := range value {
					opts.FeatureFlags.Store(key, val)
				}
				return nil
			},
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("feature"), terragruntPrefixControl),
		),
	}
}

// NewConfigFlag creates a flag for specifying the Terragrunt config file path.
func NewConfigFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&cli.GenericFlag[string]{
			Name:        ConfigFlagName,
			EnvVars:     tgPrefix.EnvVars(ConfigFlagName),
			Destination: &opts.TerragruntConfigPath,
			Usage:       "The path to the Terragrunt config file. Default is terragrunt.hcl.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("config"), terragruntPrefixControl),
	)
}

// NewDownloadDirFlag creates a flag for specifying the download directory path.
func NewDownloadDirFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		},
		flags.WithDeprecatedEnvVars(
			append(
				terragruntPrefix.EnvVars("download"),
				terragruntPrefix.EnvVars("download-dir")...,
			),
			terragruntPrefixControl,
		),
	)
}

// NewAuthProviderCmdFlag creates a flag for specifying the auth provider command.
func NewAuthProviderCmdFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&cli.GenericFlag[string]{
			Name:        AuthProviderCmdFlagName,
			EnvVars:     tgPrefix.EnvVars(AuthProviderCmdFlagName),
			Destination: &opts.AuthProviderCmd,
			Usage:       "Run the provided command and arguments to authenticate Terragrunt dynamically when necessary.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("auth-provider-cmd"), terragruntPrefixControl),
	)
}

// NewInputsDebugFlag creates a flag for enabling inputs debug output.
func NewInputsDebugFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&cli.BoolFlag{
			Name:        InputsDebugFlagName,
			EnvVars:     tgPrefix.EnvVars(InputsDebugFlagName),
			Destination: &opts.Debug,
			Usage:       "Write debug.tfvars to working folder to help root-cause issues.",
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("debug"), terragruntPrefixControl),
	)
}

// NewIAMAssumeRoleFlags creates flags for IAM assume role configuration.
func NewIAMAssumeRoleFlags(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return cli.Flags{
		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        IAMAssumeRoleFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleFlagName),
				Destination: &opts.IAMRoleOptions.RoleARN,
				Usage:       "Assume the specified IAM role before executing OpenTofu/Terraform.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-role"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[int64]{
				Name:        IAMAssumeRoleDurationFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleDurationFlagName),
				Destination: &opts.IAMRoleOptions.AssumeRoleDuration,
				Usage:       "Session duration for IAM Assume Role session.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-assume-role-duration"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        IAMAssumeRoleSessionNameFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleSessionNameFlagName),
				Destination: &opts.IAMRoleOptions.AssumeRoleSessionName,
				Usage:       "Name for the IAM Assumed Role session.",
			},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("iam-assume-role-session-name"), terragruntPrefixControl),
		),

		flags.NewFlag(
			&cli.GenericFlag[string]{
				Name:        IAMAssumeRoleWebIdentityTokenFlagName,
				EnvVars:     tgPrefix.EnvVars(IAMAssumeRoleWebIdentityTokenFlagName),
				Destination: &opts.IAMRoleOptions.WebIdentityToken,
				Usage:       "For AssumeRoleWithWebIdentity, the WebIdentity token.",
			},
			flags.WithDeprecatedEnvVars(
				append(
					terragruntPrefix.EnvVars("iam-web-identity-token"),
					terragruntPrefix.EnvVars("iam-assume-role-web-identity-token")...,
				),
				terragruntPrefixControl,
			),
		),
	}
}

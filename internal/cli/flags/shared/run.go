package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// NoAutoRetryFlagName is the flag name for `--no-auto-retry`, which turns off re-running a command
	// after a transient error.
	NoAutoRetryFlagName = "no-auto-retry"
	// NoAutoApproveFlagName is the flag name for `--no-auto-approve`, which stops a run over several units
	// from appending -auto-approve to the tofu command.
	NoAutoApproveFlagName = "no-auto-approve"
	// NoRunHooksFlagName is the flag name for `--no-hooks` on commands that run units, where it skips
	// Terragrunt hooks. [NoHooksFlagName] is the scaffolding flag of the same name.
	NoRunHooksFlagName = "no-hooks"
	// TFForwardStdoutFlagName is the flag name for `--tf-forward-stdout`, which prints tofu output as is,
	// outside the Terragrunt log.
	TFForwardStdoutFlagName = "tf-forward-stdout"
	// VersionManagerFileNameFlagName is the flag name for `--version-manager-file-name`, the version
	// manager file names hashed into the cache key.
	VersionManagerFileNameFlagName = "version-manager-file-name"
	// OutDirFlagName is the flag name for `--out-dir`, the directory plan files are saved to.
	OutDirFlagName = "out-dir"
	// JSONOutDirFlagName is the flag name for `--json-out-dir`, the directory JSON plan files are saved
	// to.
	JSONOutDirFlagName = "json-out-dir"
	// DisableCommandValidationFlagName is the flag name for the hidden, deprecated
	// `--disable-command-validation`. Setting it only evaluates its strict control.
	DisableCommandValidationFlagName = "disable-command-validation"
	// NoDestroyDependenciesCheckFlagName is the flag name for the hidden, deprecated
	// `--no-destroy-dependencies-check`. Setting it only evaluates its strict control.
	NoDestroyDependenciesCheckFlagName = "no-destroy-dependencies-check"
	// DestroyDependenciesCheckFlagName is the flag name for `--destroy-dependencies-check`, which checks
	// for dependent units before a destroy.
	DestroyDependenciesCheckFlagName = "destroy-dependencies-check"
)

// NewNoAutoRetryFlag creates the flag that turns off re-running a command after a transient error.
func NewNoAutoRetryFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoAutoRetryFlagName,
		EnvVars:     tgPrefix.EnvVars(NoAutoRetryFlagName),
		Destination: &opts.AutoRetry,
		Usage:       "Don't automatically re-run command in case of transient errors.",
		Negative:    true,
	},
		flags.WithDeprecatedFlag(&clihelper.BoolFlag{
			EnvVars: terragruntPrefix.EnvVars("auto-retry"),
		}, nil, opts.StrictControls))
}

// NewNoAutoApproveFlag creates the flag that stops a run over several units from appending -auto-approve
// to the tofu command.
func NewNoAutoApproveFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoAutoApproveFlagName,
		EnvVars:     tgPrefix.EnvVars(NoAutoApproveFlagName),
		Destination: &opts.RunAllAutoApprove,
		Usage:       "Don't automatically append '-auto-approve' to the underlying OpenTofu/Terraform commands run with 'run --all'.",
		Negative:    true,
	},
		flags.WithDeprecatedFlag(&clihelper.BoolFlag{
			EnvVars: terragruntPrefix.EnvVars("auto-approve"),
		}, nil, opts.StrictControls))
}

// NewNoRunHooksFlag creates the flag that skips Terragrunt hooks.
func NewNoRunHooksFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoRunHooksFlagName,
		EnvVars:     tgPrefix.EnvVars(NoRunHooksFlagName),
		Destination: &opts.NoRunHooks,
		Usage:       "Disable Terragrunt hooks during run.",
	})
}

// NewTFForwardStdoutFlag creates the flag that prints tofu output as is, outside the Terragrunt log.
func NewTFForwardStdoutFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return flags.NewFlag(
		&clihelper.BoolFlag{
			Name:        TFForwardStdoutFlagName,
			EnvVars:     tgPrefix.EnvVars(TFForwardStdoutFlagName),
			Destination: &opts.ForwardTFStdout,
			Usage:       "If specified, the output of OpenTofu/Terraform commands will be printed as is, without being integrated into the Terragrunt log.",
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars("forward-tf-stdout"),
			opts.StrictControls,
		),
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars("include-module-prefix"),
			opts.StrictControls,
			controls.LegacyLogs,
		),
	)
}

// NewVersionManagerFileNameFlag creates the flag that sets the version manager file names hashed into the
// cache key.
func NewVersionManagerFileNameFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return flags.NewFlag(&clihelper.SliceFlag[string]{
		Name:        VersionManagerFileNameFlagName,
		EnvVars:     tgPrefix.EnvVars(VersionManagerFileNameFlagName),
		Destination: &opts.VersionManagerFileName,
		Usage:       "File names used during the computation of the cache key for the version manager files.",
	})
}

// NewOutDirFlags creates the flags that set the directories plan files are saved to.
func NewOutDirFlags(opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        OutDirFlagName,
			EnvVars:     tgPrefix.EnvVars(OutDirFlagName),
			Destination: &opts.OutputFolder,
			Usage:       "Directory to store plan files.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("out-dir"), opts.StrictControls)),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        JSONOutDirFlagName,
				EnvVars:     tgPrefix.EnvVars(JSONOutDirFlagName),
				Destination: &opts.JSONOutputFolder,
				Usage:       "Directory to store json plan files.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("json-out-dir"),
				opts.StrictControls,
			),
		),
	}
}

// NewDisableCommandValidationFlag creates the hidden, deprecated flag whose only effect is evaluating its
// strict control.
func NewDisableCommandValidationFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return flags.NewFlag(
		&clihelper.BoolFlag{
			Name:        DisableCommandValidationFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableCommandValidationFlagName),
			Destination: &opts.DisableCommandValidation,
			Usage:       "When this flag is set, Terragrunt will not validate the tofu/terraform command.",
			Hidden:      true,
			Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
				if value {
					return opts.StrictControls.FilterByNames(controls.DisableCommandValidation).
						Evaluate(ctx)
				}

				return nil
			},
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars("disable-command-validation"),
			opts.StrictControls,
		),
	)
}

// NewDestroyDependenciesCheckFlags creates the flags that control the check for dependent units before a
// destroy.
func NewDestroyDependenciesCheckFlags(opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}

	return clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:    NoDestroyDependenciesCheckFlagName,
			EnvVars: tgPrefix.EnvVars(NoDestroyDependenciesCheckFlagName),
			Usage:   "When this flag is set, Terragrunt will not check for dependent units when destroying.",
			Hidden:  true,
			Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
				if value {
					return opts.StrictControls.FilterByNames(controls.NoDestroyDependenciesCheck).
						Evaluate(ctx)
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DestroyDependenciesCheckFlagName,
			EnvVars:     tgPrefix.EnvVars(DestroyDependenciesCheckFlagName),
			Destination: &opts.DestroyDependenciesCheck,
			Usage:       "When this flag is set, Terragrunt will check for dependent units when destroying.",
		}),
	}
}

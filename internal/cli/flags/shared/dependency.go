package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// NoDependencyOutputsFlagName is the flag name for `--no-dependency-outputs`, which skips resolving
	// dependency outputs.
	NoDependencyOutputsFlagName = "no-dependency-outputs"
	// DependencyFetchOutputFromStateFlagName is the flag name for the deprecated
	// `--dependency-fetch-output-from-state`. Reading dependency outputs from the state file is the
	// default, so setting it only evaluates its strict control.
	DependencyFetchOutputFromStateFlagName = "dependency-fetch-output-from-state"
	// NoDependencyFetchOutputFromStateFlagName is the flag name for
	// `--no-dependency-fetch-output-from-state`, which reads dependency outputs by running `tofu output`
	// in place of reading the state file.
	NoDependencyFetchOutputFromStateFlagName = "no-dependency-fetch-output-from-state"
	// UsePartialParseConfigCacheFlagName is the flag name for `--use-partial-parse-config-cache`, which
	// caches includes during partial parsing.
	UsePartialParseConfigCacheFlagName = "use-partial-parse-config-cache"
)

// NewDependencyOutputFlags creates the flags that control how `dependency` blocks resolve their outputs.
func NewDependencyOutputFlags(opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoDependencyOutputsFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDependencyOutputsFlagName),
			Destination: &opts.SkipOutput,
			Usage:       "Skip all dependency output resolution. Dependency blocks will not call tofu/terraform output.",
		}),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    DependencyFetchOutputFromStateFlagName,
				EnvVars: tgPrefix.EnvVars(DependencyFetchOutputFromStateFlagName),
				Usage:   "Read dependency outputs directly from the state file. Enabled by default; retained for backwards compatibility.",
				Action: func(ctx context.Context, _ *clihelper.Context, val bool) error {
					if val {
						return opts.StrictControls.FilterByNames(controls.DependencyFetchOutputFromState).
							Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("fetch-dependency-output-from-state"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoDependencyFetchOutputFromStateFlagName,
			EnvVars:     tgPrefix.EnvVars(NoDependencyFetchOutputFromStateFlagName),
			Destination: &opts.NoDependencyFetchOutputFromState,
			Usage:       "Read dependency outputs by running tofu/terraform output instead of reading the state file directly.",
		}),
	}
}

// NewUsePartialParseConfigCacheFlag creates the flag that caches includes during partial parsing.
func NewUsePartialParseConfigCacheFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return flags.NewFlag(
		&clihelper.BoolFlag{
			Name:        UsePartialParseConfigCacheFlagName,
			EnvVars:     tgPrefix.EnvVars(UsePartialParseConfigCacheFlagName),
			Destination: &opts.UsePartialParseConfigCache,
			Usage:       "Enables caching of includes during partial parsing operations. Will also be used for the --iam-role option if provided.",
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars("use-partial-parse-config-cache"),
			opts.StrictControls,
		),
	)
}

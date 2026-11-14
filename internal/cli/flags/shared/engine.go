package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// EngineEnableFlagName is the flag name for the hidden `--experimental-engine`, which enables the
	// iac-engine experiment.
	EngineEnableFlagName = "experimental-engine"
	// EngineCachePathFlagName is the flag name for the hidden `--engine-cache-path`, where IaC engine
	// files are cached.
	EngineCachePathFlagName = "engine-cache-path"
	// EngineSkipCheckFlagName is the flag name for the hidden `--engine-skip-check`, which skips
	// verifying the checksum of IaC engine files.
	EngineSkipCheckFlagName = "engine-skip-check"
	// EngineLogLevelFlagName is the flag name for the hidden `--engine-log-level`, the IaC engine log
	// level.
	EngineLogLevelFlagName = "engine-log-level"
	// NoEngineFlagName is the flag name for the hidden `--no-engine`, which turns off IaC engines even
	// when the iac-engine experiment is enabled.
	NoEngineFlagName = "no-engine"
)

// NewEngineFlags creates the hidden flags that configure IaC engines.
func NewEngineFlags(opts *options.TerragruntOptions) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	return clihelper.Flags{
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    EngineEnableFlagName,
				EnvVars: tgPrefix.EnvVars(EngineEnableFlagName),
				Usage:   "Enable the iac-engine experiment to use IaC engines.",
				Hidden:  true,
				Action: func(_ context.Context, _ *clihelper.Context, val bool) error {
					if val {
						return opts.Experiments.EnableExperiment(experiment.IacEngine)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("experimental-engine"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        EngineCachePathFlagName,
				EnvVars:     tgPrefix.EnvVars(EngineCachePathFlagName),
				Destination: &opts.EngineOptions.CachePath,
				Usage:       "Cache path for Terragrunt engine files.",
				Hidden:      true,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("engine-cache-path"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        EngineSkipCheckFlagName,
				EnvVars:     tgPrefix.EnvVars(EngineSkipCheckFlagName),
				Destination: &opts.EngineOptions.SkipChecksumCheck,
				Usage:       "Skip checksum check for Terragrunt engine files.",
				Hidden:      true,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("engine-skip-check"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        EngineLogLevelFlagName,
				EnvVars:     tgPrefix.EnvVars(EngineLogLevelFlagName),
				Destination: &opts.EngineOptions.LogLevel,
				Usage:       "Terragrunt engine log level.",
				Hidden:      true,
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("engine-log-level"),
				opts.StrictControls,
			),
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoEngineFlagName,
			EnvVars:     tgPrefix.EnvVars(NoEngineFlagName),
			Destination: &opts.EngineOptions.NoEngine,
			Usage:       "Disable IaC engines even when the iac-engine experiment is enabled.",
			Hidden:      true,
		}),
	}
}

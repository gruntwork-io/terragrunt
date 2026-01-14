package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	BackendBootstrapFlagName        = "backend-bootstrap"
	BackendRequireBootstrapFlagName = "backend-require-bootstrap"
	DisableBucketUpdateFlagName     = "disable-bucket-update"
)

// NewBackendFlags defines backend-related flags that should be available to both `run` and `backend` commands.
func NewBackendFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        BackendBootstrapFlagName,
			EnvVars:     tgPrefix.EnvVars(BackendBootstrapFlagName),
			Destination: &opts.BackendBootstrap,
			Usage:       "Automatically bootstrap backend infrastructure before attempting to use it.",
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        BackendRequireBootstrapFlagName,
			EnvVars:     tgPrefix.EnvVars(BackendRequireBootstrapFlagName),
			Destination: &opts.FailIfBucketCreationRequired,
			Usage:       "When this flag is set Terragrunt will fail if the remote state bucket needs to be created.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("fail-on-state-bucket-creation"), terragruntPrefixControl),
		),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        DisableBucketUpdateFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableBucketUpdateFlagName),
			Destination: &opts.DisableBucketUpdate,
			Usage:       "When this flag is set Terragrunt will not update the remote state bucket.",
		},
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("disable-bucket-update"), terragruntPrefixControl),
		),
	}
}

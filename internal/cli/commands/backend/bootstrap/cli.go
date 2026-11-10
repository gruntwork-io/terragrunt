package bootstrap

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const CommandName = "bootstrap"

func NewFlags(opts *options.TerragruntOptions) clihelper.Flags {
	backendFlags := shared.NewBackendFlags(opts, nil)
	featureFlags := shared.NewFeatureFlags(opts, nil)

	sharedFlags := make(clihelper.Flags, 0, 2+len(backendFlags)+len(featureFlags))
	sharedFlags = append(sharedFlags,
		shared.NewConfigFlag(opts, nil),
		shared.NewDownloadDirFlag(opts, nil),
	)
	sharedFlags = append(sharedFlags, backendFlags...)
	sharedFlags = append(sharedFlags, featureFlags...)

	flags.DeprecateEnvVarPrefix(sharedFlags, flags.Prefix{flags.TgPrefix, flags.TgPrefix}, opts.StrictControls)

	return sharedFlags
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	cmdFlags := NewFlags(opts)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil), shared.NewFailFastFlag(opts))

	cmd := &clihelper.Command{
		Name:  CommandName,
		Usage: "Bootstrap OpenTofu/Terraform backend infrastructure.",
		Flags: cmdFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}

package print

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt info print` command.
	CommandName = "print"
)

// NewFlags returns the flags of `terragrunt info print`, bound to opts: the
// ones that reach config parsing and the source download, and the ones that
// select units for --all.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) clihelper.Flags {
	cmdFlags := clihelper.Flags{
		shared.NewConfigFlag(opts, nil),
		shared.NewDownloadDirFlag(opts, nil),
		shared.NewTFPathFlag(opts),
		shared.NewTFForwardStdoutFlag(opts),
		shared.NewVersionManagerFileNameFlag(opts),
		shared.NewNoAutoInitFlag(opts, nil),
		shared.NewNoAutoRetryFlag(opts),
		shared.NewNoRunHooksFlag(opts),
		shared.NewSourceFlag(opts, nil),
		shared.NewSourceMapFlag(opts, nil),
		shared.NewSourceUpdateFlag(opts),
		shared.NewInputsDebugFlag(opts, nil),
		shared.NewUsePartialParseConfigCacheFlag(opts),
		shared.NewAuthProviderCmdFlag(opts, nil),
		shared.NewNoDiscoveryAuthProviderCmdFlag(opts, nil),
		shared.NewFailFastFlag(opts),
		shared.NewParallelismFlag(opts),
	}

	cmdFlags = cmdFlags.Add(shared.NewDependencyOutputFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewProviderCacheFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewEngineFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewBackendFlags(opts, nil)...)
	cmdFlags = cmdFlags.Add(shared.NewFeatureFlags(opts, nil)...)
	cmdFlags = cmdFlags.Add(shared.NewIAMAssumeRoleFlags(opts, nil)...)
	cmdFlags = cmdFlags.Add(shared.NewSelectionFlags(l, opts, v)...)
	cmdFlags = cmdFlags.Add(shared.NewCASFlags(opts, nil)...)

	return append(cmdFlags.Sort(), shared.NewAllFlag(opts, nil))
}

// NewCommand returns the `terragrunt info print` command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	cmd := &clihelper.Command{
		Name:      CommandName,
		Usage:     "Print out a short description of Terragrunt context.",
		UsageText: "terragrunt info print",
		Flags:     NewFlags(l, opts, v),
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, s, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}

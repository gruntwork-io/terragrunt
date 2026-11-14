package run

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// GraphRootFlagName is the name of the flag that sets the directory the
// dependency graph is built from.
const GraphRootFlagName = "graph-root"

// NewFlags returns the flags of `terragrunt run`, bound to opts, apart from
// --all and --graph, which [NewCommand] adds.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	cmdFlags := clihelper.Flags{
		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        GraphRootFlagName,
				EnvVars:     tgPrefix.EnvVars(GraphRootFlagName),
				Destination: &opts.GraphRoot,
				Usage:       "Root directory from where to build graph dependencies.",
			},
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("graph-root"),
				opts.StrictControls,
			),
		),

		shared.NewConfigFlag(opts, prefix),
		shared.NewDownloadDirFlag(opts, prefix),
		shared.NewTFPathFlag(opts),
		shared.NewTFForwardStdoutFlag(opts),
		shared.NewVersionManagerFileNameFlag(opts),
		shared.NewNoAutoInitFlag(opts, prefix),
		shared.NewNoAutoRetryFlag(opts),
		shared.NewNoAutoApproveFlag(opts),
		shared.NewNoRunHooksFlag(opts),
		shared.NewNoStackGenerateFlag(opts),
		shared.NewSourceFlag(opts, prefix),
		shared.NewSourceMapFlag(opts, prefix),
		shared.NewSourceUpdateFlag(opts),
		shared.NewInputsDebugFlag(opts, prefix),
		shared.NewUsePartialParseConfigCacheFlag(opts),
		shared.NewAuthProviderCmdFlag(opts, prefix),
		shared.NewNoDiscoveryAuthProviderCmdFlag(opts, prefix),
		shared.NewUnitsThatIncludeFlag(opts),
		shared.NewDisableCommandValidationFlag(opts),
		shared.NewFailFastFlag(opts),
		shared.NewParallelismFlag(opts),
	}

	cmdFlags = cmdFlags.Add(shared.NewOutDirFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewDependencyOutputFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewDestroyDependenciesCheckFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewProviderCacheFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewEngineFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewReportFlags(opts)...)
	cmdFlags = cmdFlags.Add(shared.NewBackendFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewFeatureFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewIAMAssumeRoleFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewQueueRunFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewQueueSelectionFlags(opts, prefix)...)
	cmdFlags = cmdFlags.Add(shared.NewFilterFlags(l, opts, v)...)
	cmdFlags = cmdFlags.Add(shared.NewCASFlags(opts, prefix)...)

	return cmdFlags.Sort()
}

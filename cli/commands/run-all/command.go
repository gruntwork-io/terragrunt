package runall

import (
	"context"
	"sort"

	"github.com/gruntwork-io/terragrunt/cli/commands"
	awsproviderpatch "github.com/gruntwork-io/terragrunt/cli/commands/aws-provider-patch"
	graphdependencies "github.com/gruntwork-io/terragrunt/cli/commands/graph-dependencies"
	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	renderjson "github.com/gruntwork-io/terragrunt/cli/commands/render-json"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	validateinputs "github.com/gruntwork-io/terragrunt/cli/commands/validate-inputs"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "run-all"

	FlagNameTerragruntProviderCache        = "terragrunt-provider-cache"
	FlagNameTerragruntProviderCacheDir     = "terragrunt-provider-cache-dir"
	FlagNameTerragruntProviderCompleteLock = "terragrunt-provider-complete-lock"
	FlagNameTerragruntRegistryHostname     = "terragrunt-registry-hostname"
	FlagNameTerragruntRegistryPort         = "terragrunt-registry-port"
	FlagNameTerragruntRegistryToken        = "terragrunt-registry-token"
	FlagNameTerragruntRegistryNames        = "terragrunt-registry-names"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	globalFlags := commands.NewGlobalFlags(opts)
	globalFlags.Add(
		&cli.BoolFlag{
			Name:        FlagNameTerragruntProviderCache,
			Destination: &opts.ProviderCache,
			EnvVar:      "TERRAGRUNT_PROVIDER_CACHE",
			Usage:       "Enables provider caching.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntProviderCacheDir,
			Destination: &opts.ProviderCacheDir,
			EnvVar:      "TERRAGRUNT_PROVIDER_CACHE_DIR",
			Usage:       "The path to the cache directory. Default is .terragrunt-cache/provider-cache in the working directory.",
		},
		&cli.BoolFlag{
			Name:        FlagNameTerragruntProviderCompleteLock,
			Destination: &opts.ProviderCompleteLock,
			EnvVar:      "TERRAGRUNT_PROVIDER_COMPLETE_LOCK",
			Usage:       "Disables terraform 'plugin_cache_may_break_dependency_lock_file' feature.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntRegistryToken,
			Destination: &opts.RegistryToken,
			EnvVar:      "TERRAGRUNT_REGISTRY_TOKEN",
			Usage:       "The Token for connecting to the built-in Private Registry server. By default it is generated automatically.",
		},
		&cli.GenericFlag[string]{
			Name:        FlagNameTerragruntRegistryHostname,
			Destination: &opts.RegistryHostname,
			EnvVar:      "TERRAGRUNT_REGISTRY_HOSTNAME",
			Usage:       "The hostname of the built-in Private Registry server. Default is 'localhsot'.",
		},
		&cli.GenericFlag[int]{
			Name:        FlagNameTerragruntRegistryPort,
			Destination: &opts.RegistryPort,
			EnvVar:      "TERRAGRUNT_REGISTRY_PORT",
			Usage:       "The port of the built-in Private Registry server. Default is '5758'.",
		},
		&cli.SliceFlag[string]{
			Name:        FlagNameTerragruntRegistryNames,
			Destination: &opts.RegistryNames,
			EnvVar:      "TERRAGRUNT_REGISTRY_NAMES",
			Usage:       "The list of registries to cache. Default is 'registry.terraform.io'.",
		})
	return globalFlags
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:        CommandName,
		Usage:       "Run a terraform command against a 'stack' by running the specified command in each subfolder.",
		Description: "The command will recursively find terragrunt modules in the current directory tree and run the terraform command in dependency order (unless the command is destroy, in which case the command is run in reverse dependency order).",
		Flags:       NewFlags(opts).Sort(),
		Subcommands: subCommands(opts).SkipRunning(),
		Action:      action(opts),
	}
}

func action(opts *options.TerragruntOptions) func(ctx *cli.Context) error {
	return func(cliCtx *cli.Context) error {
		opts.RunTerragrunt = func(ctx context.Context, opts *options.TerragruntOptions) error {
			if cmd := cliCtx.Command.Subcommand(opts.TerraformCommand); cmd != nil {
				cliCtx := cliCtx.WithValue(options.ContextKey, opts)
				return cmd.Action(cliCtx)
			}
			return terraform.Run(ctx, opts)
		}

		if opts.ProviderCache {
			return RunWithProviderCache(cliCtx, opts.OptionsFromContext(cliCtx))
		}
		return Run(cliCtx, opts.OptionsFromContext(cliCtx))
	}
}

func subCommands(opts *options.TerragruntOptions) cli.Commands {
	cmds := cli.Commands{
		terragruntinfo.NewCommand(opts),    // terragrunt-info
		validateinputs.NewCommand(opts),    // validate-inputs
		graphdependencies.NewCommand(opts), // graph-dependencies
		hclfmt.NewCommand(opts),            // hclfmt
		renderjson.NewCommand(opts),        // render-json
		awsproviderpatch.NewCommand(opts),  // aws-provider-patch
	}

	sort.Sort(cmds)

	// add terraform command `*` after sorting to put the command at the end of the list in the help.
	cmds.Add(terraform.NewCommand(opts))

	return cmds
}

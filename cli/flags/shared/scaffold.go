package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"
	NoShellFlagName       = "no-shell"
	NoHooksFlagName       = "no-hooks"
)

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

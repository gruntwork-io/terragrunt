package shared

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	RootFileNameFlagName  = "root-file-name"
	NoIncludeRootFlagName = "no-include-root"
	NoShellFlagName       = "no-shell"
	NoHooksFlagName       = "no-hooks"
)

// NewScaffoldingFlags creates the flags shared between catalog and scaffold commands.
func NewScaffoldingFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        RootFileNameFlagName,
			EnvVars:     tgPrefix.EnvVars(RootFileNameFlagName),
			Destination: &opts.ScaffoldRootFileName,
			Usage:       "Name of the root Terragrunt configuration file, if used.",
			Action: func(ctx context.Context, _ *clihelper.Context, value string) error {
				if value == "" {
					return clihelper.NewExitError("root-file-name flag cannot be empty", clihelper.ExitCodeGeneralError)
				}

				if value != opts.TerragruntConfigPath {
					opts.ScaffoldRootFileName = value

					return nil
				}

				if err := opts.StrictControls.FilterByNames("RootTerragruntHCL").Evaluate(ctx); err != nil {
					return clihelper.NewExitError(err, clihelper.ExitCodeGeneralError)
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoIncludeRootFlagName,
			EnvVars:     tgPrefix.EnvVars(NoIncludeRootFlagName),
			Destination: &opts.ScaffoldNoIncludeRoot,
			Usage:       "Do not include root unit in scaffolding done by catalog.",
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoShellFlagName,
			EnvVars:     tgPrefix.EnvVars(NoShellFlagName),
			Destination: &opts.NoShell,
			Usage:       "Disable shell commands when using boilerplate templates.",
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoHooksFlagName,
			EnvVars:     tgPrefix.EnvVars(NoHooksFlagName),
			Destination: &opts.NoHooks,
			Usage:       "Disable hooks when using boilerplate templates.",
		}),
	}
}

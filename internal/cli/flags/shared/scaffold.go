package shared

import (
	"context"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// RootFileNameFlagName is the flag name for `--root-file-name`, the file name of the root Terragrunt
	// config that scaffolded units include.
	RootFileNameFlagName = "root-file-name"
	// NoIncludeRootFlagName is the flag name for `--no-include-root`, which leaves the root config include
	// out of scaffolded units.
	NoIncludeRootFlagName = "no-include-root"
	// NoShellFlagName is the flag name for `--no-shell`, which turns off shell commands in boilerplate
	// templates.
	NoShellFlagName = "no-shell"
	// NoHooksFlagName is the flag name for `--no-hooks`, which turns off hooks in boilerplate templates.
	NoHooksFlagName = "no-hooks"
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
					return clihelper.NewExitError(
						"root-file-name flag cannot be empty",
						clihelper.ExitCodeGeneralError,
					)
				}

				if filepath.Clean(value) != config.DefaultTerragruntConfigPath {
					opts.ScaffoldRootFileName = value

					return nil
				}

				if err := opts.StrictControls.FilterByNames(controls.RootTerragruntHCL).
					Evaluate(ctx); err != nil {
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

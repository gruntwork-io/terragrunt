// Package renderjson provides the command to render the final terragrunt config, with all variables, includes, and functions resolved, as json.
package renderjson

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "render-json"

	OutFlagName                     = "out"
	WithMetadataFlagName            = "with-metadata"
	DisableDependentModulesFlagName = "disable-dependent-modules"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	cliRedesignControl := flags.StrictControlsByGroup(opts.StrictControls, CommandName, controls.CLIRedesign)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutFlagName,
			EnvVars:     tgPrefix.EnvVars(OutFlagName),
			Destination: &opts.JSONOut,
			Usage:       "The file path that terragrunt should use when rendering the terragrunt.hcl config as json.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix.Append("json"), cliRedesignControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        WithMetadataFlagName,
			EnvVars:     tgPrefix.EnvVars(WithMetadataFlagName),
			Destination: &opts.RenderJSONWithMetadata,
			Usage:       "Add metadata to the rendered JSON file.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableDependentModulesFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableDependentModulesFlagName),
			Destination: &opts.JSONDisableDependentModules,
			Usage:       "Disable identification of dependent modules rendering json config.",
		},
			flags.WithDeprecatedPrefix(terragruntPrefix.Append("json"), cliRedesignControl)),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}

	return &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, as json.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       append(run.NewFlags(opts, nil), NewFlags(opts, prefix)...).Sort(),
		Action:      func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}

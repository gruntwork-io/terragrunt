// Package render provides the command to render the final terragrunt config in various formats.
package render

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "render"

	FormatFlagName                  = "format"
	WriteFlagName                   = "write"
	OutFlagName                     = "out"
	WithMetadataFlagName            = "with-metadata"
	DisableDependentModulesFlagName = "disable-dependent-modules"
)

func NewFlags(opts *Options, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "The output format to render the config in. Currently supports: json",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        WriteFlagName,
			EnvVars:     tgPrefix.EnvVars(WriteFlagName),
			Destination: &opts.Write,
			Usage:       "Write the rendered config to a file.",
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutFlagName,
			EnvVars:     tgPrefix.EnvVars(OutFlagName),
			Destination: &opts.JSONOut,
			Usage:       "The file name that terragrunt should use when rendering the terragrunt.hcl config (next to the unit configuration).",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        WithMetadataFlagName,
			EnvVars:     tgPrefix.EnvVars(WithMetadataFlagName),
			Destination: &opts.RenderJSONWithMetadata,
			Usage:       "Add metadata to the rendered output file.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableDependentModulesFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableDependentModulesFlagName),
			Destination: &opts.JSONDisableDependentModules,
			Usage:       "Disable identification of dependent modules when rendering config.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}
	renderOpts := NewOptions(opts)

	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Render the final terragrunt config, with all variables, includes, and functions resolved, in the specified format.",
		Description:          "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:                append(run.NewFlags(opts, nil), NewFlags(renderOpts, prefix)...),
		Action:               func(ctx *cli.Context) error { return Run(ctx, renderOpts) },
		ErrorOnUndefinedFlag: true,
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}

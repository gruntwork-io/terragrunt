// Package render provides the command to render the final terragrunt config in various formats.
package render

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "render"

	FormatFlagName                  = "format"
	JSONFlagName                    = "json"
	WriteFlagName                   = "write"
	WriteAliasFlagName              = "w"
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
			Action: func(ctx *cli.Context, value string) error {
				// Set the default output path based on the format.
				switch value {
				case FormatJSON:
					if opts.OutputPath == "" {
						opts.OutputPath = "terragrunt.rendered.json"
					}

					return nil
				case FormatHCL:
					if opts.OutputPath == "" {
						opts.OutputPath = "terragrunt.rendered.hcl"
					}

					return nil
				default:
					return errors.New("invalid format: " + value)
				}
			},
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:    JSONFlagName,
			EnvVars: tgPrefix.EnvVars(JSONFlagName),
			Usage:   "Render the config in JSON format. Equivalent to --format=json.",
			Action: func(ctx *cli.Context, value bool) error {
				opts.Format = FormatJSON

				if opts.OutputPath == "" {
					opts.OutputPath = "terragrunt.rendered.json"
				}

				return nil
			},
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        WriteFlagName,
			EnvVars:     tgPrefix.EnvVars(WriteFlagName),
			Aliases:     []string{WriteAliasFlagName},
			Destination: &opts.Write,
			Usage:       "Write the rendered config to a file.",
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutFlagName,
			EnvVars:     tgPrefix.EnvVars(OutFlagName),
			Destination: &opts.OutputPath,
			Usage:       "The file name that terragrunt should use when rendering the terragrunt.hcl config (next to the unit configuration).",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        WithMetadataFlagName,
			EnvVars:     tgPrefix.EnvVars(WithMetadataFlagName),
			Destination: &opts.RenderMetadata,
			Usage:       "Add metadata to the rendered output file.",
		}),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableDependentModulesFlagName,
			EnvVars:     tgPrefix.EnvVars(DisableDependentModulesFlagName),
			Destination: &opts.DisableDependentModules,
			Usage:       "Disable identification of dependent modules when rendering config.",
		}),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	prefix := flags.Prefix{CommandName}
	renderOpts := NewOptions(opts)

	cmd := &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, in the specified format.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       append(run.NewFlags(opts, nil), NewFlags(renderOpts, prefix)...),
		Action: func(ctx *cli.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)
			renderOpts := renderOpts.Clone()
			renderOpts.TerragruntOptions = tgOpts

			return Run(ctx, renderOpts)
		},
		ErrorOnUndefinedFlag: true,
	}

	cmd = runall.WrapCommand(opts, cmd)

	return cmd
}

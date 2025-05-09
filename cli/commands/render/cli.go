// Package render provides the command to render the final terragrunt config in various formats.
package render

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
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

func NewFlags(opts *Options, cmdPrefix flags.Name) cli.Flags {
	strictControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	return cli.Flags{
		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(FormatFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(FormatFlagName),
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
			Name:      JSONFlagName,
			EnvVars:   flags.EnvVarsWithTgPrefix(JSONFlagName),
			ConfigKey: cmdPrefix.ConfigKey(JSONFlagName),
			Usage:     "Render the config in JSON format. Equivalent to --format=json.",
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
			EnvVars:     flags.EnvVarsWithTgPrefix(WriteFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(WriteFlagName),
			Aliases:     []string{WriteAliasFlagName},
			Destination: &opts.Write,
			Usage:       "Write the rendered config to a file.",
		}),

		flags.NewFlag(&cli.GenericFlag[string]{
			Name:        OutFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(OutFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(OutFlagName),
			Destination: &opts.OutputPath,
			Usage:       "The file name that terragrunt should use when rendering the terragrunt.hcl config (next to the unit configuration).",
		},
			flags.WithDeprecatedFlagName("json-out", strictControl),                                   // `--json-out`
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("render-json-out"), strictControl),  // `TG_RENDER_JSON_OUT`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("json-out"), strictControl), // `--terragrunt-json-out`, `TERRAGRUNT_JSON_OUT`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        WithMetadataFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(WithMetadataFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(WithMetadataFlagName),
			Destination: &opts.RenderMetadata,
			Usage:       "Add metadata to the rendered output file.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("render-json-with-metadata"), strictControl), // `TG_RENDER_JSON_WITH_METADATA`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("with-metadata"), strictControl),     // `--terragrunt-with-metadata`, `TERRAGRUNT_WITH_METADATA`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        DisableDependentModulesFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(DisableDependentModulesFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(DisableDependentModulesFlagName),
			Destination: &opts.DisableDependentModules,
			Usage:       "Disable identification of dependent modules when rendering config.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("render-json-disable-dependent-modules"), strictControl),  // `TG_RENDER_JSON_DISABLE_DEPENDENT_MODULES`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("json-disable-dependent-modules"), strictControl), // `--terragrunt-json-disable-dependent-modules`, `TERRAGRUNT_JSON_DISABLE_DEPENDENT_MODULES`
		),
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmdPrefix := flags.Name{CommandName}
	renderOpts := NewOptions(opts)

	cmd := &cli.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, in the specified format.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       append(run.NewFlags(opts), NewFlags(renderOpts, cmdPrefix)...),
		Action: func(ctx *cli.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)
			renderOpts := renderOpts.Clone()
			renderOpts.TerragruntOptions = tgOpts

			return Run(ctx, renderOpts)
		},
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)
	// TODO: For backward compatibility, remove after getting rid of the `render-json` command, as supporting the `graph` flag for the `render` command is pointless.
	cmd = graph.WrapCommand(opts, cmd, run.Run)

	return cmd
}

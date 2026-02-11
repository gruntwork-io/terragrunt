// Package render provides the command to render the final terragrunt config in various formats.
package render

import (
	"context"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
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

func NewFlags(opts *Options, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "The output format to render the config in. Currently supports: json",
			Action: func(_ context.Context, _ *clihelper.Context, value string) error {
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

		flags.NewFlag(&clihelper.BoolFlag{
			Name:    JSONFlagName,
			EnvVars: tgPrefix.EnvVars(JSONFlagName),
			Usage:   "Render the config in JSON format. Equivalent to --format=json.",
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				opts.Format = FormatJSON

				if opts.OutputPath == "" {
					opts.OutputPath = "terragrunt.rendered.json"
				}

				return nil
			},
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        WriteFlagName,
			EnvVars:     tgPrefix.EnvVars(WriteFlagName),
			Aliases:     []string{WriteAliasFlagName},
			Destination: &opts.Write,
			Usage:       "Write the rendered config to a file.",
		}),

		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        OutFlagName,
			EnvVars:     tgPrefix.EnvVars(OutFlagName),
			Destination: &opts.OutputPath,
			Usage:       "The file name that terragrunt should use when rendering the terragrunt.hcl config (next to the unit configuration).",
		},
			flags.WithDeprecatedFlagName("json-out", terragruntPrefixControl),                          // `--json-out` (deprecated: use `--out` instead)
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("render-json-out"), terragruntPrefixControl),  // `TG_RENDER_JSON_OUT`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("json-out"), terragruntPrefixControl), // `TERRAGRUNT_JSON_OUT`
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:        WithMetadataFlagName,
			EnvVars:     tgPrefix.EnvVars(WithMetadataFlagName),
			Destination: &opts.RenderMetadata,
			Usage:       "Add metadata to the rendered output file.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("render-json-with-metadata"), terragruntPrefixControl), // `TG_RENDER_JSON_WITH_METADATA`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("with-metadata"), terragruntPrefixControl),     // `TERRAGRUNT_WITH_METADATA`
		),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:    DisableDependentModulesFlagName,
			EnvVars: tgPrefix.EnvVars(DisableDependentModulesFlagName),
			Hidden:  true,
			Usage:   "Deprecated: Disable identification of dependent modules when rendering config. This flag has no effect as dependent modules discovery has been removed.",
			Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
				if value {
					return opts.StrictControls.FilterByNames(controls.DisableDependentModules).Evaluate(ctx)
				}

				return nil
			},
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("render-json-disable-dependent-modules"), terragruntPrefixControl),  // `TG_RENDER_JSON_DISABLE_DEPENDENT_MODULES`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("json-disable-dependent-modules"), terragruntPrefixControl), // `TERRAGRUNT_JSON_DISABLE_DEPENDENT_MODULES`
		),
	}
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *clihelper.Command {
	prefix := flags.Prefix{CommandName}
	renderOpts := NewOptions(opts)

	cmdFlags := append(runcmd.NewFlags(l, opts, nil), NewFlags(renderOpts, prefix)...)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, prefix))

	cmd := &clihelper.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, in the specified format.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       cmdFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)

			clonedOpts := renderOpts.Clone()
			clonedOpts.TerragruntOptions = tgOpts

			return Run(ctx, l, clonedOpts)
		},
	}

	return cmd
}

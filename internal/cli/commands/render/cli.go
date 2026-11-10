package render

import (
	"context"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt render` command.
	CommandName = "render"

	// FormatFlagName is the name of the flag that selects the output format:
	// [FormatHCL] or [FormatJSON].
	FormatFlagName = "format"
	// JSONFlagName is the name of the flag that is shorthand for --format=json.
	JSONFlagName = "json"
	// WriteFlagName is the name of the flag that writes the rendered config to
	// a file in place of stdout.
	WriteFlagName = "write"
	// WriteAliasFlagName is the short alias of the --write flag.
	WriteAliasFlagName = "w"
	// OutFlagName is the name of the flag that sets the file --write writes to,
	// relative to the unit's config directory.
	OutFlagName = "out"
	// WithMetadataFlagName is the name of the flag that records, in JSON
	// output, the file that set each value.
	WithMetadataFlagName = "with-metadata"
	// DisableDependentModulesFlagName is the name of the hidden, deprecated flag
	// that has no effect. Setting it evaluates its strict control.
	DisableDependentModulesFlagName = "disable-dependent-modules"
)

// NewFlags returns the flags of `terragrunt render`, bound to opts.
func NewFlags(opts *Options) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	renderFlags := clihelper.Flags{
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        FormatFlagName,
			EnvVars:     tgPrefix.EnvVars(FormatFlagName),
			Destination: &opts.Format,
			Usage:       "The output format to render the config in. Supported values: hcl, json.",
			Action: func(_ context.Context, _ *clihelper.Context, value string) error {
				return validateFormat(value)
			},
		}),

		flags.NewFlag(&clihelper.BoolFlag{
			Name:    JSONFlagName,
			EnvVars: tgPrefix.EnvVars(JSONFlagName),
			Usage:   "Render the config in JSON format. Equivalent to --format=json.",
			Action: func(_ context.Context, _ *clihelper.Context, _ bool) error {
				opts.Format = FormatJSON

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

		flags.NewFlag(
			&clihelper.GenericFlag[string]{
				Name:        OutFlagName,
				EnvVars:     tgPrefix.EnvVars(OutFlagName),
				Destination: &opts.OutputPath,
				Usage:       "The file name that terragrunt should use when rendering the terragrunt.hcl config (next to the unit configuration).",
			},
			flags.WithDeprecatedFlagName(
				"json-out",
				opts.StrictControls,
			), // `--json-out` (deprecated: use `--out` instead)
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("render-json-out"),
				opts.StrictControls,
			), // `TG_RENDER_JSON_OUT`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("json-out"),
				opts.StrictControls,
			), // `TERRAGRUNT_JSON_OUT`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        WithMetadataFlagName,
				EnvVars:     tgPrefix.EnvVars(WithMetadataFlagName),
				Destination: &opts.RenderMetadata,
				Usage:       "Add metadata to the rendered output file.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("render-json-with-metadata"),
				opts.StrictControls,
			), // `TG_RENDER_JSON_WITH_METADATA`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("with-metadata"),
				opts.StrictControls,
			), // `TERRAGRUNT_WITH_METADATA`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:    DisableDependentModulesFlagName,
				EnvVars: tgPrefix.EnvVars(DisableDependentModulesFlagName),
				Hidden:  true,
				Usage:   "Deprecated: Disable identification of dependent modules when rendering config. This flag has no effect as dependent modules discovery has been removed.",
				Action: func(ctx context.Context, _ *clihelper.Context, value bool) error {
					if value {
						return opts.StrictControls.FilterByNames(controls.DisableDependentModules).
							Evaluate(ctx)
					}

					return nil
				},
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("render-json-disable-dependent-modules"),
				opts.StrictControls,
			), // `TG_RENDER_JSON_DISABLE_DEPENDENT_MODULES`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("json-disable-dependent-modules"),
				opts.StrictControls,
			), // `TERRAGRUNT_JSON_DISABLE_DEPENDENT_MODULES`
		),
		shared.NewAllFlag(opts.TerragruntOptions, nil),
	}

	flags.DeprecateEnvVarPrefix(renderFlags, flags.Prefix{flags.TgPrefix, CommandName}, opts.StrictControls)

	return renderFlags
}

// NewCommand returns the `terragrunt render` command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) *clihelper.Command {
	renderOpts := NewOptions(opts)

	cmdFlags := append(runcmd.NewFlags(l, opts, v, nil), NewFlags(renderOpts)...)

	cmd := &clihelper.Command{
		Name:        CommandName,
		Usage:       "Render the final terragrunt config, with all variables, includes, and functions resolved, in the specified format.",
		Description: "This is useful for enforcing policies using static analysis tools like Open Policy Agent, or for debugging your terragrunt config.",
		Flags:       cmdFlags,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)

			clonedOpts := renderOpts.Clone()
			clonedOpts.TerragruntOptions = tgOpts

			return Run(ctx, l, v, clonedOpts)
		},
	}

	return cmd
}

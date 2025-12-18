package validate

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/internal/runner/graph"
	"github.com/gruntwork-io/terragrunt/internal/runner/runall"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	CommandName = "validate"

	StrictFlagName         = "strict"
	InputsFlagName         = "inputs"
	ShowConfigPathFlagName = "show-config-path"
	JSONFlagName           = "json"
)

func NewFlags(l log.Logger, opts *options.TerragruntOptions) cli.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flagSet := cli.Flags{
		flags.NewFlag(
			&cli.BoolFlag{
				Name:        StrictFlagName,
				EnvVars:     tgPrefix.EnvVars(StrictFlagName),
				Destination: &opts.HCLValidateStrict,
				Usage:       "Enables strict mode. When used in combination with the `--inputs` flag, any inputs defined in Terragrunt that are _not_ used in OpenTofu/Terraform will trigger an error.",
			},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars(
				"strict-validate",             // `TG_STRICT_VALIDATE`
				"hclvalidate-strict-validate", // `TG_HCLVALIDATE_STRICT_VALIDATE`
			), terragruntPrefixControl),
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("strict-validate"), terragruntPrefixControl), // `TERRAGRUNT_STRICT_VALIDATE`
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        InputsFlagName,
				EnvVars:     tgPrefix.EnvVars(InputsFlagName),
				Destination: &opts.HCLValidateInputs,
				Usage:       "Checks if the Terragrunt configured inputs align with OpenTofu/Terraform defined variables.",
			},
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        ShowConfigPathFlagName,
				EnvVars:     tgPrefix.EnvVars(ShowConfigPathFlagName),
				Usage:       "Emit a list of files with invalid configurations after validating all configurations.",
				Destination: &opts.HCLValidateShowConfigPath,
			},

			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclvalidate-strict-validate"), terragruntPrefixControl),          // `TG_HCLVALIDATE_STRICT_VALIDATE`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclvalidate-show-config-path"), terragruntPrefixControl), // `TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH`
		),

		flags.NewFlag(
			&cli.BoolFlag{
				Name:        JSONFlagName,
				EnvVars:     tgPrefix.EnvVars(JSONFlagName),
				Destination: &opts.HCLValidateJSONOutput,
				Usage:       "Format results in JSON format.",
			},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclvalidate-json"), terragruntPrefixControl),         // `TG_HCLVALIDATE_JSON`
			flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("hclvalidate-json"), terragruntPrefixControl), // `TERRAGRUNT_HCLVALIDATE_JSON`
		),

		shared.NewTFPathFlag(opts),
	}

	flagSet = flagSet.Add(shared.NewQueueFlags(opts, nil)...)
	flagSet = flagSet.Add(shared.NewFilterFlags(l, opts)...)

	return flagSet
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmdFlags := NewFlags(l, opts)
	cmdFlags = append(cmdFlags, shared.NewAllFlag(opts, nil), shared.NewGraphFlag(opts, nil))

	cmd := &cli.Command{
		Name:                         CommandName,
		Usage:                        "Recursively find HashiCorp Configuration Language (HCL) files and validate them.",
		Flags:                        cmdFlags,
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			tgOpts := opts.OptionsFromContext(ctx)
			tgOpts.SummaryDisable = true

			if tgOpts.RunAll {
				return runall.Run(ctx.Context, l, tgOpts)
			}

			if tgOpts.Graph {
				return graph.Run(ctx.Context, l, tgOpts)
			}

			return Run(ctx, l, tgOpts)
		},
	}

	return cmd
}

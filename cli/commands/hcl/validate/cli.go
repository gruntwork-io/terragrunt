package validate

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
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

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flagSet := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        StrictFlagName,
			EnvVars:     tgPrefix.EnvVars(StrictFlagName),
			Destination: &opts.HCLValidateStrict,
			Usage:       "Enables strict mode. When used in combination with the `--inputs` flag, any inputs defined in Terragrunt that are _not_ used in OpenTofu/Terraform will trigger an error.",
		},
			flags.WithDeprecatedFlagName("strict-validate", terragruntPrefixControl), // `--strict-validate`
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars(
				"strict-validate",             // `TG_STRICT_VALIDATE`
				"hclvalidate-strict-validate", // `TG_HCLVALIDATE_STRICT_VALIDATE`
			), terragruntPrefixControl),
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("strict-validate"), terragruntPrefixControl), // `--terragrunt-strict-validate`, `TERRAGRUNT_STRICT_VALIDATE`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputsFlagName,
			EnvVars:     tgPrefix.EnvVars(InputsFlagName),
			Destination: &opts.HCLValidateInputs,
			Usage:       "Checks if the Terragrunt configured inputs align with OpenTofu/Terraform defined variables.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowConfigPathFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowConfigPathFlagName),
			Usage:       "Emit a list of files with invalid configurations after validating all configurations.",
			Destination: &opts.HCLValidateShowConfigPath,
		},

			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclvalidate-strict-validate"), terragruntPrefixControl),          // `TG_HCLVALIDATE_STRICT_VALIDATE`
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclvalidate-show-config-path"), terragruntPrefixControl), // `--terragrunt-hclvalidate-show-config-path`, `TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Destination: &opts.HCLValidateJSONOutput,
			Usage:       "Format results in JSON format.",
		},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars("hclvalidate-json"), terragruntPrefixControl),         // `TG_HCLVALIDATE_JSON`
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclvalidate-json"), terragruntPrefixControl), // `--terragrunt-hclvalidate-json`, `TERRAGRUNT_HCLVALIDATE_JSON`
		),
	}

	return flagSet
}

func NewCommand(l log.Logger, opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:                         CommandName,
		Usage:                        "Recursively find HashiCorp Configuration Language (HCL) files and validate them.",
		Flags:                        NewFlags(opts, nil),
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, l, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(l, opts, cmd, run.Run, true)
	cmd = graph.WrapCommand(l, opts, cmd, run.Run, true)

	return cmd
}

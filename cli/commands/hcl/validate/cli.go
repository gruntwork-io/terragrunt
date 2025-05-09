package validate

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "validate"

	StrictFlagName         = "strict"
	InputsFlagName         = "inputs"
	ShowConfigPathFlagName = "show-config-path"
	JSONFlagName           = "json"
)

func NewFlags(opts *options.TerragruntOptions, cmdPrefix flags.Name) cli.Flags {
	strictControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flagSet := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        StrictFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(StrictFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(StrictFlagName),
			Destination: &opts.HCLValidateStrict,
			Usage:       "Enables strict mode. When used in combination with the `--inputs` flag, any inputs defined in Terragrunt that are _not_ used in OpenTofu/Terraform will trigger an error.",
		},
			flags.WithDeprecatedFlagName("strict-validate", strictControl), // `--strict-validate`
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix(
				"strict-validate",             // `TG_STRICT_VALIDATE`
				"hclvalidate-strict-validate", // `TG_HCLVALIDATE_STRICT_VALIDATE`
			), strictControl),
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("strict-validate"), strictControl), // `--terragrunt-strict-validate`, `TERRAGRUNT_STRICT_VALIDATE`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputsFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(InputsFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(InputsFlagName),
			Destination: &opts.HCLValidateInputs,
			Usage:       "Checks if the Terragrunt configured inputs align with OpenTofu/Terraform defined variables.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowConfigPathFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(ShowConfigPathFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(ShowConfigPathFlagName),
			Usage:       "Emit a list of files with invalid configurations after validating all configurations.",
			Destination: &opts.HCLValidateShowConfigPath,
		},

			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclvalidate-strict-validate"), strictControl),          // `TG_HCLVALIDATE_STRICT_VALIDATE`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("hclvalidate-show-config-path"), strictControl), // `--terragrunt-hclvalidate-show-config-path`, `TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH`
		),

		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     flags.EnvVarsWithTgPrefix(JSONFlagName),
			ConfigKey:   cmdPrefix.ConfigKey(JSONFlagName),
			Destination: &opts.HCLValidateJSONOutput,
			Usage:       "Format results in JSON format.",
		},
			flags.WithDeprecatedEnvVars(flags.EnvVarsWithTgPrefix("hclvalidate-json"), strictControl),         // `TG_HCLVALIDATE_JSON`
			flags.WithDeprecatedNames(flags.FlagNamesWithTerragruntPrefix("hclvalidate-json"), strictControl), // `--terragrunt-hclvalidate-json`, `TERRAGRUNT_HCLVALIDATE_JSON`
		),
	}

	return flagSet
}

func NewCommand(opts *options.TerragruntOptions, cmdPrefix flags.Name) *cli.Command {
	cmdPrefix = cmdPrefix.Append(CommandName)

	cmd := &cli.Command{
		Name:                         CommandName,
		Usage:                        "Recursively find HashiCorp Configuration Language (HCL) files and validate them.",
		Flags:                        NewFlags(opts, cmdPrefix),
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd, run.Run)
	cmd = graph.WrapCommand(opts, cmd, run.Run)

	return cmd
}

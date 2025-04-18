package validate

import (
	"github.com/gruntwork-io/terragrunt/cli/commands/common/graph"
	"github.com/gruntwork-io/terragrunt/cli/commands/common/runall"
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	CommandName = "validate"

	StrictFlagName         = "strict"
	InputFlagName          = "input"
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
			flags.WithDeprecatedNames(tgPrefix.FlagNames("strict-validate"), terragruntPrefixControl),
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("strict-validate"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputFlagName,
			EnvVars:     tgPrefix.EnvVars(InputFlagName),
			Destination: &opts.HCLValidateInput,
			Usage:       "Checks if the Terragrunt configured inputs align with OpenTofu/Terraform defined variables.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowConfigPathFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowConfigPathFlagName),
			Usage:       "Emit a list of files with invalid configurations after validating all configurations.",
			Destination: &opts.HCLValidateShowConfigPath,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclvalidate-show-config-path"), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Destination: &opts.HCLValidateJSONOutput,
			Usage:       "Format results in JSON format.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames("hclvalidate-json"), terragruntPrefixControl)),
	}

	return flagSet
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	cmd := &cli.Command{
		Name:                 CommandName,
		Usage:                "Recursively find HashiCorp Configuration Language (HCL) files and validate them.",
		Flags:                NewFlags(opts, nil),
		ErrorOnUndefinedFlag: true,
		Action: func(ctx *cli.Context) error {
			return Run(ctx, opts.OptionsFromContext(ctx))
		},
	}

	cmd = runall.WrapCommand(opts, cmd)
	cmd = graph.WrapCommand(opts, cmd)

	return cmd
}

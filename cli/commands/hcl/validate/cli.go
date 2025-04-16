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

	DeprecatedHclvalidateShowConfigPathFlagName = "hclvalidate-show-config-path"
	DeprecatedHclvalidateJSONFlagName           = "hclvalidate-json"
	DeprecatedStrictValidateFlagName            = "strict-validate"
)

func NewFlags(opts *options.TerragruntOptions, prefix flags.Prefix) cli.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByCommand(opts.StrictControls, CommandName)

	flags := cli.Flags{
		flags.NewFlag(&cli.BoolFlag{
			Name:        StrictFlagName,
			EnvVars:     tgPrefix.EnvVars(StrictFlagName),
			Destination: &opts.HCLValidateStrict,
			Usage:       "Sets strict mode. By default, strict mode is off.",
		},
			flags.WithDeprecatedNames(tgPrefix.FlagNames(DeprecatedStrictValidateFlagName), terragruntPrefixControl),
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedStrictValidateFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        InputFlagName,
			EnvVars:     tgPrefix.EnvVars(InputFlagName),
			Destination: &opts.HCLValidateInput,
			Usage:       "Checks if the terragrunt configured inputs align with the terraform defined variables.",
		}),
		flags.NewFlag(&cli.BoolFlag{
			Name:        ShowConfigPathFlagName,
			EnvVars:     tgPrefix.EnvVars(ShowConfigPathFlagName),
			Usage:       "Show a list of files with invalid configuration.",
			Destination: &opts.HCLValidateShowConfigPath,
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclvalidateShowConfigPathFlagName), terragruntPrefixControl)),

		flags.NewFlag(&cli.BoolFlag{
			Name:        JSONFlagName,
			EnvVars:     tgPrefix.EnvVars(JSONFlagName),
			Destination: &opts.HCLValidateJSONOutput,
			Usage:       "Output the result in JSON format.",
		},
			flags.WithDeprecatedNames(terragruntPrefix.FlagNames(DeprecatedHclvalidateJSONFlagName), terragruntPrefixControl)),
	}

	return flags
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

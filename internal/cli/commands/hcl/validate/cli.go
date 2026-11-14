package validate

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	rewrite "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// CommandName is the name of the `terragrunt hcl validate` command.
	CommandName = "validate"

	// StrictFlagName is the name of the flag that, with --inputs, also fails on
	// inputs the module does not declare.
	StrictFlagName = "strict"
	// InputsFlagName is the name of the flag that checks each unit's inputs
	// against the variables its module declares.
	InputsFlagName = "inputs"
	// OutputsFlagName is the name of the flag that checks the outputs contracts of each unit.
	OutputsFlagName = "outputs"
	// ShowConfigPathFlagName is the name of the flag that prints the paths of
	// the files with invalid configuration.
	ShowConfigPathFlagName = "show-config-path"
	// JSONFlagName is the name of the flag that prints diagnostics as JSON.
	JSONFlagName = "json"
)

// NewFlags returns the flags of `terragrunt hcl validate`, bound to opts.
func NewFlags(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv) clihelper.Flags {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}

	flagSet := clihelper.Flags{
		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        StrictFlagName,
				EnvVars:     tgPrefix.EnvVars(StrictFlagName),
				Destination: &opts.HCLValidateStrict,
				Usage:       "Enables strict mode. When used in combination with the --inputs flag, any inputs defined in Terragrunt that are _not_ used in OpenTofu/Terraform will trigger an error.",
			},
			flags.WithDeprecatedEnvVars(tgPrefix.EnvVars(
				"strict-validate",             // `TG_STRICT_VALIDATE`
				"hclvalidate-strict-validate", // `TG_HCLVALIDATE_STRICT_VALIDATE`
			), opts.StrictControls),
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("strict-validate"),
				opts.StrictControls,
			), // `TERRAGRUNT_STRICT_VALIDATE`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        InputsFlagName,
				EnvVars:     tgPrefix.EnvVars(InputsFlagName),
				Destination: &opts.HCLValidateInputs,
				Usage:       "Checks if the Terragrunt configured inputs align with OpenTofu/Terraform defined variables.",
			},
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        OutputsFlagName,
				EnvVars:     tgPrefix.EnvVars(OutputsFlagName),
				Destination: &opts.HCLValidateOutputs,
				Usage: flags.ExperimentUsage(
					opts.Experiments,
					experiment.OutputsBlock,
					"Checks that each output a unit declares exists in its OpenTofu/Terraform module and that its dependents read only declared outputs.",
				),
			},
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        ShowConfigPathFlagName,
				EnvVars:     tgPrefix.EnvVars(ShowConfigPathFlagName),
				Usage:       "Emit a list of files with invalid configurations after validating all configurations.",
				Destination: &opts.HCLValidateShowConfigPath,
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclvalidate-show-config-path"),
				opts.StrictControls,
			), // `TG_HCLVALIDATE_SHOW_CONFIG_PATH`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("hclvalidate-show-config-path"),
				opts.StrictControls,
			), // `TERRAGRUNT_HCLVALIDATE_SHOW_CONFIG_PATH`
		),

		flags.NewFlag(
			&clihelper.BoolFlag{
				Name:        JSONFlagName,
				EnvVars:     tgPrefix.EnvVars(JSONFlagName),
				Destination: &opts.HCLValidateJSONOutput,
				Usage:       "Format results in JSON format.",
			},
			flags.WithDeprecatedEnvVars(
				tgPrefix.EnvVars("hclvalidate-json"),
				opts.StrictControls,
			), // `TG_HCLVALIDATE_JSON`
			flags.WithDeprecatedEnvVars(
				terragruntPrefix.EnvVars("hclvalidate-json"),
				opts.StrictControls,
			), // `TERRAGRUNT_HCLVALIDATE_JSON`
		),

		shared.NewTFPathFlag(opts),
	}

	flagSet = flagSet.Add(shared.NewQueueRunFlags(opts, nil)...)
	flagSet = flagSet.Add(shared.NewQueueSelectionFlags(opts, nil)...)
	flagSet = flagSet.Add(shared.NewFilterFlags(l, opts, v)...)

	return flagSet
}

// NewCommand returns the `terragrunt hcl validate` command.
func NewCommand(l log.Logger, opts *options.TerragruntOptions, v *venv.Venv, s *rewrite.Store) *clihelper.Command {
	cmd := &clihelper.Command{
		Name:                         CommandName,
		Usage:                        "Recursively find HashiCorp Configuration Language (HCL) files and validate them.",
		Flags:                        NewFlags(l, opts, v),
		DisabledErrorOnUndefinedFlag: true,
		Action: func(ctx context.Context, _ *clihelper.Context) error {
			return Run(ctx, l, v, s, opts.OptionsFromContext(ctx))
		},
	}

	return cmd
}

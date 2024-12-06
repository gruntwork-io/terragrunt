//nolint:unparam
package cli

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
)

// The following flags are DEPRECATED
const (
	TerragruntIncludeModulePrefixFlagName = "terragrunt-include-module-prefix"
	TerragruntIncludeModulePrefixEnvName  = "TERRAGRUNT_INCLUDE_MODULE_PREFIX"

	TerragruntDisableLogFormattingFlagName = "terragrunt-disable-log-formatting"
	TerragruntDisableLogFormattingEnvName  = "TERRAGRUNT_DISABLE_LOG_FORMATTING"

	TerragruntJSONLogFlagName = "terragrunt-json-log"
	TerragruntJSONLogEnvName  = "TERRAGRUNT_JSON_LOG"

	TerragruntTfLogJSONFlagName = "terragrunt-tf-logs-to-json"
	TerragruntTfLogJSONEnvName  = "TERRAGRUNT_TF_JSON_LOG"
)

// NewDeprecatedFlags creates and returns deprecated flags.
func NewDeprecatedFlags(opts *options.TerragruntOptions) cli.Flags {
	flags := cli.Flags{
		&cli.BoolFlag{
			Name:   TerragruntIncludeModulePrefixFlagName,
			EnvVar: TerragruntIncludeModulePrefixEnvName,
			Usage:  "When this flag is set output from Terraform sub-commands is prefixed with module path.",
			Hidden: true,
			Action: func(ctx *cli.Context, _ bool) error {
				opts.Logger.Warnf("The %q flag is deprecated. Use the functionality-inverted %q flag instead. By default, Terraform/OpenTofu output is integrated into the Terragrunt log, which prepends additional data, such as timestamps and prefixes, to log entries.", TerragruntIncludeModulePrefixFlagName, commands.TerragruntForwardTFStdoutFlagName)
				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntDisableLogFormattingFlagName,
			EnvVar:      TerragruntDisableLogFormattingEnvName,
			Destination: &opts.DisableLogFormatting,
			Usage:       "If specified, logs will be displayed in key/value format. By default, logs are formatted in a human readable format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.SetFormat(format.NewKeyValueFormat())

				if control, ok := strict.GetStrictControl(strict.DisableLogFormatting); ok {
					warn, triggered, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					if !triggered {
						opts.Logger.Warnf(warn)
					}
				}

				return nil
			},
		},
		&cli.BoolFlag{
			Name:        TerragruntJSONLogFlagName,
			EnvVar:      TerragruntJSONLogEnvName,
			Destination: &opts.JSONLogFormat,
			Usage:       "If specified, Terragrunt will output its logs in JSON format.",
			Hidden:      true,
			Action: func(_ *cli.Context, _ bool) error {
				opts.LogFormatter.SetFormat(format.NewJSONFormat())

				if control, ok := strict.GetStrictControl(strict.JSONLog); ok {
					warn, triggered, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					if !triggered {
						opts.Logger.Warnf(warn)
					}
				}

				return nil
			},
		},
		&cli.BoolFlag{
			Name:   TerragruntTfLogJSONFlagName,
			EnvVar: TerragruntTfLogJSONEnvName,
			Usage:  "If specified, Terragrunt will wrap Terraform stdout and stderr in JSON.",
			Hidden: true,
			Action: func(_ *cli.Context, _ bool) error {
				if control, ok := strict.GetStrictControl(strict.JSONLog); ok {
					warn, triggered, err := control.Evaluate(opts)
					if err != nil {
						return err
					}

					if !triggered {
						opts.Logger.Warnf(warn)
					}
				}

				return nil
			},
		},
	}

	return flags
}

//nolint:unparam
package cli

import (
	"github.com/gruntwork-io/terragrunt/cli/commands"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

// The following flags are DEPRECATED
const (
	TerragruntIncludeModulePrefixFlagName = "terragrunt-include-module-prefix"
	TerragruntIncludeModulePrefixEnvName  = "TERRAGRUNT_INCLUDE_MODULE_PREFIX"
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
	}

	return flags
}

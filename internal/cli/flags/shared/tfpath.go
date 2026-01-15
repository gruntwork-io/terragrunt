package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	TFPathFlagName = "tf-path"
)

// NewTFPathFlag creates a flag for specifying the OpenTofu/Terraform binary path.
func NewTFPathFlag(opts *options.TerragruntOptions) *flags.Flag {
	tgPrefix := flags.Prefix{flags.TgPrefix}
	terragruntPrefix := flags.Prefix{flags.TerragruntPrefix}
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(
		&clihelper.GenericFlag[string]{
			Name:    TFPathFlagName,
			EnvVars: tgPrefix.EnvVars(TFPathFlagName),
			Usage:   "Path to the OpenTofu/Terraform binary. Default is tofu (on PATH).",
			Setter: func(value string) error {
				opts.TFPath = value
				opts.TFPathExplicitlySet = true
				return nil
			},
		},
		flags.WithDeprecatedEnvVars(terragruntPrefix.EnvVars("tfpath"), terragruntPrefixControl),
	)
}

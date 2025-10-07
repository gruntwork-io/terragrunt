// Package shared provides flags that are shared by multiple commands.
//
// This package is underutilized right now, as some more serious refactoring is needed to make sure all
// shared flags use this package instead of re-using flags from other commands.
package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	TFPathFlagName = "tf-path"
)

// NewTFPathFlag creates a flag for specifying the OpenTofu/Terraform binary path.
func NewTFPathFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)
	terragruntPrefixControl := flags.StrictControlsByGlobalFlags(opts.StrictControls)

	return flags.NewFlag(
		&cli.GenericFlag[string]{
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

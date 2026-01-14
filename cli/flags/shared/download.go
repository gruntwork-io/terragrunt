package shared

import (
	"github.com/gruntwork-io/terragrunt/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	DownloadDirFlagName = "download-dir"
)

// NewDownloadDirFlag creates a flag for specifying the download directory path.
func NewDownloadDirFlag(opts *options.TerragruntOptions, prefix flags.Prefix, commandName string) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	var terragruntPrefixControl flags.RegisterStrictControlsFunc
	if commandName != "" {
		terragruntPrefixControl = flags.StrictControlsByCommand(opts.StrictControls, commandName)
	} else {
		terragruntPrefixControl = flags.StrictControlsByGlobalFlags(opts.StrictControls)
	}

	return flags.NewFlag(
		&cli.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		},
		flags.WithDeprecatedEnvVars(
			append(
				terragruntPrefix.EnvVars("download"),
				terragruntPrefix.EnvVars("download-dir")...,
			),
			terragruntPrefixControl,
		),
	)
}

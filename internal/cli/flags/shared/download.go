package shared

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	DownloadDirFlagName = "download-dir"
)

// NewDownloadDirFlag creates a flag for specifying the download directory path.
func NewDownloadDirFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return flags.NewFlag(
		&clihelper.GenericFlag[string]{
			Name:        DownloadDirFlagName,
			EnvVars:     tgPrefix.EnvVars(DownloadDirFlagName),
			Destination: &opts.DownloadDir,
			Usage:       "The path to download OpenTofu/Terraform modules into. Default is .terragrunt-cache in the working directory.",
		},
		flags.WithDeprecatedEnvVars(
			slices.Concat(
				terragruntPrefix.EnvVars("download"),
				terragruntPrefix.EnvVars("download-dir"),
			),
			opts.StrictControls,
		),
	)
}

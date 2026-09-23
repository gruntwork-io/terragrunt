package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	SourceFlagName    = "source"
	SourceMapFlagName = "source-map"
)

// NewSourceFlag creates a flag for downloading configuration from a source URL instead of the configured one.
func NewSourceFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return flags.NewFlag(
		&clihelper.GenericFlag[string]{
			Name:        SourceFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceFlagName),
			Destination: &opts.Source,
			Usage:       "Download OpenTofu/Terraform configurations from the specified source into a temporary folder, and run Terraform in that temporary folder.",
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars(SourceFlagName),
			opts.StrictControls,
		),
	)
}

// NewSourceMapFlag creates a flag for replacing matching source URLs with the mapped destination.
func NewSourceMapFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)
	terragruntPrefix := prefix.Prepend(flags.TerragruntPrefix)

	return flags.NewFlag(
		&clihelper.MapFlag[string, string]{
			Name:        SourceMapFlagName,
			EnvVars:     tgPrefix.EnvVars(SourceMapFlagName),
			Destination: &opts.SourceMap,
			Usage:       "Replace any source URL (including the source URL of a config pulled in with dependency blocks) that has root source with dest.",
			Splitter:    util.SplitUrls,
		},
		flags.WithDeprecatedEnvVars(
			terragruntPrefix.EnvVars(SourceMapFlagName),
			opts.StrictControls,
		),
	)
}

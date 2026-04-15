package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	NoCASFlagName = "no-cas"
)

// NewNoCASFlag creates the --no-cas flag for disabling CAS even when the experiment is enabled.
func NewNoCASFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&clihelper.BoolFlag{
		Name:        NoCASFlagName,
		EnvVars:     tgPrefix.EnvVars(NoCASFlagName),
		Destination: &opts.NoCAS,
		Usage:       "Disable the CAS (Content Addressable Storage) feature even when the experiment is enabled.",
	})
}

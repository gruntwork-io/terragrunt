package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	NoCASFlagName         = "no-cas"
	CASCloneDepthFlagName = "cas-clone-depth"
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

// NewCASCloneDepthFlag creates the --cas-clone-depth flag for CAS git clone depth.
func NewCASCloneDepthFlag(opts *options.TerragruntOptions, prefix flags.Prefix) *flags.Flag {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return flags.NewFlag(&clihelper.GenericFlag[int]{
		Name:        CASCloneDepthFlagName,
		EnvVars:     tgPrefix.EnvVars(CASCloneDepthFlagName),
		Destination: &opts.CASCloneDepth,
		Usage:       "When using CAS, pass this value to git clone --depth (default 1; -1 clones full history). For negative values use --cas-clone-depth=-1 so the dash doesn't result in the value being parsed as a flag.",
	})
}

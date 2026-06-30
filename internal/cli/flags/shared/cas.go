package shared

import (
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// NoCASFlagName is the name of the flag that disables CAS even when the experiment is enabled.
	NoCASFlagName = "no-cas"
	// CASCloneDepthFlagName is the name of the flag that controls the git clone depth CAS uses.
	CASCloneDepthFlagName = "cas-clone-depth"
)

// NewCASFlags creates the flags controlling CAS (Content Addressable Storage)
// behavior: --no-cas to disable CAS even when the experiment is enabled, and
// --cas-clone-depth to control the git clone depth CAS uses.
func NewCASFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	return clihelper.Flags{
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        NoCASFlagName,
			EnvVars:     tgPrefix.EnvVars(NoCASFlagName),
			Destination: &opts.NoCAS,
			Usage:       "Disable the CAS (Content Addressable Storage) feature.",
		}),
		flags.NewFlag(&clihelper.GenericFlag[int]{
			Name:        CASCloneDepthFlagName,
			EnvVars:     tgPrefix.EnvVars(CASCloneDepthFlagName),
			Destination: &opts.CASCloneDepth,
			Usage:       "When using CAS, pass this value to git clone --depth (default 1; -1 clones full history). For negative values use --cas-clone-depth=-1 so the dash doesn't result in the value being parsed as a flag.",
		}),
	}
}

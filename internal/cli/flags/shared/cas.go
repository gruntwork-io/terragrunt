package shared

import (
	"context"
	"errors"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// NoCASFlagName is the name of the flag that disables CAS even when the experiment is enabled.
	NoCASFlagName = "no-cas"
	// CASCloneDepthFlagName is the name of the flag that controls the git clone depth CAS uses.
	CASCloneDepthFlagName = "cas-clone-depth"
	// CASOfflineFlagName is the name of the flag that forbids CAS from contacting a Git remote.
	CASOfflineFlagName = "cas-offline"
	// CASRefreshFlagName is the name of the flag that makes CAS ignore its persisted probe cache.
	CASRefreshFlagName = "cas-refresh"
	// CASProbeTTLFlagName is the name of the flag that sets how long CAS trusts a
	// persisted probe of a branch, HEAD, or non-version tag.
	CASProbeTTLFlagName = "cas-probe-ttl"
)

var errNegativeProbeTTL = errors.New("must not be negative")

// NewCASFlags creates the flags controlling CAS (Content Addressable Storage)
// behavior: --no-cas to disable CAS even when the experiment is enabled,
// --cas-clone-depth to control the git clone depth CAS uses, --cas-offline
// and --cas-refresh to pin or bypass the persisted probe cache, and
// --cas-probe-ttl to set how long probes of mutable refs are trusted. The last
// three are gated behind the [experiment.OfflineCAS] experiment.
func NewCASFlags(opts *options.TerragruntOptions, prefix flags.Prefix) clihelper.Flags {
	tgPrefix := prefix.Prepend(flags.TgPrefix)

	var probeTTLText string

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
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        CASOfflineFlagName,
			EnvVars:     tgPrefix.EnvVars(CASOfflineFlagName),
			Destination: &opts.CASOffline,
			Usage: flags.ExperimentUsage(opts.Experiments, experiment.OfflineCAS,
				"When using CAS, never contact a Git remote: answer every Git source from the local store and the persisted probe cache, and fail on anything missing. Other sources, such as HTTP or S3, still reach their remote."),
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				if !value {
					return nil
				}

				if !opts.Experiments.Evaluate(experiment.OfflineCAS) {
					return &CASExperimentRequiredError{FlagName: CASOfflineFlagName}
				}

				if opts.CASRefresh {
					return new(CASOfflineRefreshFlagsError)
				}

				return nil
			},
		}),
		flags.NewFlag(&clihelper.BoolFlag{
			Name:        CASRefreshFlagName,
			EnvVars:     tgPrefix.EnvVars(CASRefreshFlagName),
			Destination: &opts.CASRefresh,
			Usage: flags.ExperimentUsage(opts.Experiments, experiment.OfflineCAS,
				"When using CAS, ignore the persisted probe cache and query every source's remote again."),
			Action: func(_ context.Context, _ *clihelper.Context, value bool) error {
				if !value {
					return nil
				}

				if !opts.Experiments.Evaluate(experiment.OfflineCAS) {
					return &CASExperimentRequiredError{FlagName: CASRefreshFlagName}
				}

				if opts.CASOffline {
					return new(CASOfflineRefreshFlagsError)
				}

				return nil
			},
		}),
		flags.NewFlag(&clihelper.GenericFlag[string]{
			Name:        CASProbeTTLFlagName,
			EnvVars:     tgPrefix.EnvVars(CASProbeTTLFlagName),
			Destination: &probeTTLText,
			Usage: flags.ExperimentUsage(opts.Experiments, experiment.OfflineCAS,
				"When using CAS, trust a persisted probe of a branch, HEAD, or non-version tag for this long (for example 10m) before querying the remote again. Defaults to 0, which re-queries on every run. Version tags are trusted for 24h regardless."),
			Setter: func(value string) error {
				ttl, err := time.ParseDuration(value)
				if err != nil {
					return err
				}

				if ttl < 0 {
					return errNegativeProbeTTL
				}

				opts.CASProbeTTL = ttl

				return nil
			},
			Action: func(_ context.Context, _ *clihelper.Context, value string) error {
				// A TTL of zero is a valid value and also the default, so only the
				// raw text distinguishes a flag that was set from one that was not.
				if value == "" || opts.Experiments.Evaluate(experiment.OfflineCAS) {
					return nil
				}

				return &CASExperimentRequiredError{FlagName: CASProbeTTLFlagName}
			},
		}),
	}
}

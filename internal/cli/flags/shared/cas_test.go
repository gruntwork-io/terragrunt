package shared_test

import (
	"flag"
	"io"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags/shared"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCASProbeTTLFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    clihelper.Args
		wantTTL time.Duration
		wantErr bool
	}{
		{name: "unset", args: clihelper.Args{}, wantTTL: 0},
		{name: "minutes", args: clihelper.Args{"--cas-probe-ttl", "10m"}, wantTTL: 10 * time.Minute},
		{name: "zero", args: clihelper.Args{"--cas-probe-ttl", "0"}, wantTTL: 0},
		{name: "negative", args: clihelper.Args{"--cas-probe-ttl", "-1h"}, wantErr: true},
		{name: "not a duration", args: clihelper.Args{"--cas-probe-ttl", "soon"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var opts options.TerragruntOptions

			fs := new(flag.FlagSet)
			fs.SetOutput(io.Discard)

			for _, f := range shared.NewCASFlags(&opts, flags.Prefix{}) {
				require.NoError(t, f.Apply(fs, map[string]string{}))
			}

			err := fs.Parse(tt.args)
			if tt.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantTTL, opts.CASProbeTTL)
		})
	}
}

func TestCASProbeFlagsRequireExperiment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env      map[string]string
		name     string
		flagName string
		args     clihelper.Args
	}{
		{name: "offline", flagName: shared.CASOfflineFlagName, args: clihelper.Args{"--cas-offline"}, env: map[string]string{}},
		{name: "refresh", flagName: shared.CASRefreshFlagName, args: clihelper.Args{"--cas-refresh"}, env: map[string]string{}},
		{name: "probe ttl", flagName: shared.CASProbeTTLFlagName, args: clihelper.Args{"--cas-probe-ttl", "10m"}, env: map[string]string{}},
		{name: "zero probe ttl", flagName: shared.CASProbeTTLFlagName, args: clihelper.Args{"--cas-probe-ttl", "0"}, env: map[string]string{}},
		{name: "offline env var", flagName: shared.CASOfflineFlagName, env: map[string]string{"TG_CAS_OFFLINE": "true"}},
		{name: "refresh env var", flagName: shared.CASRefreshFlagName, env: map[string]string{"TG_CAS_REFRESH": "true"}},
		{name: "probe ttl env var", flagName: shared.CASProbeTTLFlagName, env: map[string]string{"TG_CAS_PROBE_TTL": "10m"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			casFlags := shared.NewCASFlags(opts, flags.Prefix{})

			require.NoError(t, casFlags.Parse(tt.args, tt.env))

			err := casFlags.RunActions(t.Context(), &clihelper.Context{})

			var gateErr *shared.CASExperimentRequiredError
			require.ErrorAs(t, err, &gateErr)
			assert.Equal(t, tt.flagName, gateErr.FlagName)

			require.NoError(t, opts.Experiments.EnableExperiment(experiment.OfflineCAS))
			require.NoError(t, casFlags.RunActions(t.Context(), &clihelper.Context{}))
		})
	}
}

func TestCASProbeFlagsAllowedWithoutExperimentWhenUnset(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	casFlags := shared.NewCASFlags(opts, flags.Prefix{})

	require.NoError(t, casFlags.Parse(clihelper.Args{"--no-cas", "--cas-clone-depth", "3"}, map[string]string{}))
	require.NoError(t, casFlags.RunActions(t.Context(), &clihelper.Context{}))
}

package runner_test

import (
	"errors"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// sopsLocalsHCL evaluates sops_decrypt_file during configuration parsing, the
// phase https://github.com/gruntwork-io/terragrunt/issues/6755 reports as
// unprotected by errors.retry.
const sopsLocalsHCL = `
locals {
  secret = sops_decrypt_file("secret.enc.json")
}
`

// transientSopsError mirrors the KMS/OIDC failure from issue #6755.
const transientSopsError = "error decrypting key: failed to decrypt sops data key with GCP KMS key: " +
	"rpc error: code = Unauthenticated desc = transport: per-RPC creds failed: " +
	"dial tcp 20.85.130.105:443: i/o timeout (Client.Timeout exceeded while awaiting headers)"

// flakySopsVenv returns an in-memory venv whose SOPS decrypter fails the first
// failures decrypts with a transient network error before succeeding, plus the
// decrypt call counter.
func flakySopsVenv(failures int) (*venv.Venv, *atomic.Int32) {
	var calls atomic.Int32

	v := memVenv(tfVersionOutput).WithSops(vsops.NewMemDecrypter(
		func(map[string]string, string, string) ([]byte, error) {
			if int(calls.Add(1)) <= failures {
				return nil, errors.New(transientSopsError)
			}

			return []byte(`{"value":"ok"}`), nil
		}))

	return v, &calls
}

// transientErrorsConfig retries anything containing "i/o timeout" without sleeping.
func transientErrorsConfig(maxAttempts int) *errorconfig.Config {
	return &errorconfig.Config{
		Retry: map[string]*errorconfig.RetryConfig{
			"transient": {
				Name:            "transient",
				RetryableErrors: []*errorconfig.Pattern{{Pattern: regexp.MustCompile(`(?s).*i/o timeout.*`)}},
				MaxAttempts:     maxAttempts,
			},
		},
		Ignore: map[string]*errorconfig.IgnoreConfig{},
	}
}

// TestRunnerRun_RetryParseErrorsRecoversTransientSops pins the fix for issue
// #6755: with the retry-parse-errors experiment on, a transient failure inside
// HCL evaluation is retried by the errors engine instead of failing the unit.
func TestRunnerRun_RetryParseErrorsRecoversTransientSops(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsVenv(2)
	app := newTestUnit(t, v, memRoot, "app", sopsLocalsHCL)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.Errors = transientErrorsConfig(3)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{app})
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.NoError(t, rnr.Run(t.Context(), l, v, opts, r), "the transient decrypt failures are retried away")
	// One parse attempt evaluates locals at most twice (partial + full parse), so recovering from two failures proves a re-parse.
	assert.GreaterOrEqual(t, calls.Load(), int32(3), "the two failed decrypts are retried until one succeeds")
}

// TestRunnerRun_ParseErrorsFailFastWithoutExperiment pins that without the
// experiment nothing changes: the first parse failure still fails the unit.
func TestRunnerRun_ParseErrorsFailFastWithoutExperiment(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsVenv(2)
	app := newTestUnit(t, v, memRoot, "app", sopsLocalsHCL)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	opts.Errors = transientErrorsConfig(3)

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{app})
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	runErr := rnr.Run(t.Context(), l, v, opts, r)
	require.Error(t, runErr, "without the experiment the parse failure surfaces")
	require.ErrorContains(t, runErr, "i/o timeout", "the original transient error is what surfaces")
	assert.GreaterOrEqual(t, calls.Load(), int32(1), "the failing decrypt was reached")
	// One parse attempt evaluates locals at most twice (partial + full parse); more would mean a retry.
	assert.LessOrEqual(t, calls.Load(), int32(2), "the parse is not retried")
}

// TestRunnerRun_RetryParseErrorsHonorsUnitErrorsBlock pins that the unit's own
// errors.retry rules, already partial-parsed by discovery, govern re-parses:
// the catch-all pattern from issue #6755 retries an error the built-in
// defaults would not.
func TestRunnerRun_RetryParseErrorsHonorsUnitErrorsBlock(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	v := memVenv(tfVersionOutput).WithSops(vsops.NewMemDecrypter(
		func(map[string]string, string, string) ([]byte, error) {
			if calls.Add(1) <= 2 {
				return nil, errors.New("flaky parse gremlin")
			}

			return []byte(`{"value":"ok"}`), nil
		}))

	app := newTestUnit(t, v, memRoot, "app", sopsLocalsHCL)
	app.StoreConfig(&config.TerragruntConfig{
		Errors: &config.ErrorsConfig{
			Retry: []*config.RetryBlock{{
				Label:           "all_transient_errors",
				RetryableErrors: []string{".*"},
				MaxAttempts:     3,
			}},
		},
	})

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	l := thlogger.CreateLogger()

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, component.Components{app})
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.NoError(t, rnr.Run(t.Context(), l, v, opts, r), "the unit's own catch-all retry rule applies to its re-parse")
	// One parse attempt evaluates locals at most twice (partial + full parse), so recovering from two failures proves a re-parse.
	assert.GreaterOrEqual(t, calls.Load(), int32(3), "the two failed decrypts are retried until one succeeds")
}

// TestRunnerRun_RetryParseErrorsDiscoveryHandoffEndToEnd pins the documented
// composition end to end: discovery reads the unit's errors.retry block from
// real HCL, stores it on the unit, and the runner's full parse retries with it.
func TestRunnerRun_RetryParseErrorsDiscoveryHandoffEndToEnd(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	// The discovery parse (first decrypt) succeeds so the errors block is stored; the runner's
	// full parse fails transiently once and must be retried by the discovery-read rule.
	v := memVenv(tfVersionOutput).WithSops(vsops.NewMemDecrypter(
		func(map[string]string, string, string) ([]byte, error) {
			if calls.Add(1) == 2 {
				return nil, errors.New("flaky parse gremlin")
			}

			return []byte(`{"value":"ok"}`), nil
		}))

	writeUnit(t, v, memRoot, "app", `
errors {
  retry "gremlins" {
    retryable_errors   = [".*flaky parse gremlin.*"]
    max_attempts       = 3
    sleep_interval_sec = 0
  }
}

locals {
  secret = sops_decrypt_file("secret.enc.json")
}
`)

	opts := newStackOpts(t, memRoot, tf.CommandNamePlan)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	l := thlogger.CreateLogger()

	d := discovery.NewDiscovery(memRoot).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: memRoot}).
		WithRelationships()

	components, err := d.Discover(t.Context(), l, v, opts)
	require.NoError(t, err)
	require.Len(t, components, 1)

	unit, ok := components[0].(*component.Unit)
	require.True(t, ok)
	require.NotNil(t, unit.Config(), "discovery stored the partial-parsed config on the unit")
	require.NotNil(t, unit.Config().Errors, "the stored config carries the unit's errors block")

	if unit.DiscoveryContext() == nil {
		unit.SetDiscoveryContext(&component.DiscoveryContext{WorkingDir: memRoot})
	}

	rnr, err := runner.NewFromComponents(t.Context(), l, opts, components)
	require.NoError(t, err)

	r := report.NewReport().WithWorkingDir(memRoot)

	require.NoError(
		t,
		rnr.Run(t.Context(), l, v, opts, r),
		"the discovery-read catch rule retries the full-parse failure; the defaults would not match it",
	)
	assert.GreaterOrEqual(t, calls.Load(), int32(3), "discovery decrypt, one failed full-parse decrypt, then a retried one")
}

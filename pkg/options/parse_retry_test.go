package options_test

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errTransientParse = errors.New("dial tcp 1.2.3.4:443: i/o timeout")

// retryIOTimeouts builds an errors config whose single retry rule matches "i/o timeout".
func retryIOTimeouts(maxAttempts int) *errorconfig.Config {
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

func TestRunWithParseRetry(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		configure   func(t *testing.T, opts *options.TerragruntOptions)
		assertErr   func(t *testing.T, err error)
		name        string
		failures    int
		expectCalls int
	}{
		{
			name:     "experiment off leaves the operation unretried",
			failures: 3,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = retryIOTimeouts(3)
			},
			expectCalls: 1,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTransientParse)
			},
		},
		{
			name:     "matching error is retried until the operation succeeds",
			failures: 2,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = retryIOTimeouts(3)
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 3,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.NoError(t, err)
			},
		},
		{
			name:     "non-matching error surfaces on the first attempt",
			failures: 3,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = &errorconfig.Config{
					Retry: map[string]*errorconfig.RetryConfig{
						"other": {
							Name:            "other",
							RetryableErrors: []*errorconfig.Pattern{{Pattern: regexp.MustCompile(`no such pattern`)}},
							MaxAttempts:     3,
						},
					},
				}
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 1,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTransientParse)
			},
		},
		{
			name:     "exhausted attempts return the typed max attempts error",
			failures: 10,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = retryIOTimeouts(2)
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 2,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var maxAttemptsErr *errorconfig.MaxAttemptsReachedError

				require.ErrorAs(t, err, &maxAttemptsErr)
			},
		},
		{
			name:     "no-auto-retry keeps the failure on the first attempt",
			failures: 3,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = retryIOTimeouts(3)
				opts.AutoRetry = false
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 1,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTransientParse)
			},
		},
		{
			name:     "no-auto-retry with an exhausted budget still surfaces the original error",
			failures: 3,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = retryIOTimeouts(1)
				opts.AutoRetry = false
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 1,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTransientParse, "no retry ran, so no max-attempts error may be reported")

				var maxAttemptsErr *errorconfig.MaxAttemptsReachedError

				require.NotErrorAs(t, err, &maxAttemptsErr)
			},
		},
		{
			name:     "ignore rules do not apply to parse failures",
			failures: 3,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = &errorconfig.Config{
					Ignore: map[string]*errorconfig.IgnoreConfig{
						"everything": {
							Name:            "everything",
							IgnorableErrors: []*errorconfig.Pattern{{Pattern: regexp.MustCompile(`(?s).*`)}},
						},
					},
				}
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 1,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTransientParse, "a parse that produced no configuration cannot be ignored")
			},
		},
		{
			name:     "ignore rule matching the same error does not veto the retry rule",
			failures: 2,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = retryIOTimeouts(3)
				opts.Errors.Ignore = map[string]*errorconfig.IgnoreConfig{
					"everything": {
						Name:            "everything",
						IgnorableErrors: []*errorconfig.Pattern{{Pattern: regexp.MustCompile(`(?s).*`)}},
					},
				}
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 3,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.NoError(t, err, "retry rules stay in force even when an ignore rule also matches")
			},
		},
		{
			name:     "nil errors config leaves the operation unretried",
			failures: 3,
			configure: func(t *testing.T, opts *options.TerragruntOptions) {
				t.Helper()

				opts.Errors = nil
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))
			},
			expectCalls: 1,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errTransientParse)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("/test/terragrunt.hcl")
			require.NoError(t, err)

			tc.configure(t, opts)

			calls := 0
			operationErr := opts.RunWithParseRetry(t.Context(), logger.CreateLogger(), func() error {
				calls++
				if calls <= tc.failures {
					return errTransientParse
				}

				return nil
			})

			tc.assertErr(t, operationErr)
			assert.Equal(t, tc.expectCalls, calls)
		})
	}
}

// TestRunWithParseRetryHonorsContextCancellation pins that a canceled context
// stops the retry loop instead of sleeping toward the next attempt.
func TestRunWithParseRetryHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("/test/terragrunt.hcl")
	require.NoError(t, err)

	errCfg := retryIOTimeouts(5)
	errCfg.Retry["transient"].SleepIntervalSec = 300
	opts.Errors = errCfg
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	calls := 0
	operationErr := opts.RunWithParseRetry(ctx, logger.CreateLogger(), func() error {
		calls++

		return errTransientParse
	})

	require.ErrorIs(t, operationErr, context.Canceled)
	assert.Equal(t, 1, calls, "the canceled context wins over the retry sleep")
}

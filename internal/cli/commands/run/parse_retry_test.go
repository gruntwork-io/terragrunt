package run_test

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runcmd "github.com/gruntwork-io/terragrunt/internal/cli/commands/run"
	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const memUnitDir = "/repo"

// flakySopsUnit builds an in-memory venv holding one unit whose locals decrypt
// a secret, with a SOPS decrypter that fails with a transient network error on
// exactly the calls shouldFail selects.
func flakySopsUnit(t *testing.T, shouldFail func(call int32) bool) (*venv.Venv, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32

	v := venvtest.New().
		WithHandler(func(context.Context, vexec.Invocation) vexec.Result {
			return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
		}).
		WithSops(vsops.NewMemDecrypter(func(map[string]string, string, string) ([]byte, error) {
			if shouldFail(calls.Add(1)) {
				return nil, errors.New("dial tcp 1.2.3.4:443: i/o timeout")
			}

			return []byte(`{"value":"ok"}`), nil
		}))

	require.NoError(t, v.FS.MkdirAll(memUnitDir, 0o755))
	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(memUnitDir, "terragrunt.hcl"),
		[]byte("locals {\n  secret = sops_decrypt_file(\"secret.enc.json\")\n}\n"),
		0o644,
	))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(memUnitDir, "main.tf"), []byte(""), 0o644))

	return v, &calls
}

// singleRunOpts returns options for a single-unit plan with a zero-sleep retry
// rule matching "i/o timeout".
func singleRunOpts(t *testing.T) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(memUnitDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = memUnitDir
	opts.RootWorkingDir = memUnitDir
	opts.TerraformCommand = tf.CommandNamePlan
	opts.TerraformCliArgs = iacargs.New(tf.CommandNamePlan)
	opts.Errors = &errorconfig.Config{
		Retry: map[string]*errorconfig.RetryConfig{
			"transient": {
				Name:            "transient",
				RetryableErrors: []*errorconfig.Pattern{{Pattern: regexp.MustCompile(`(?s).*i/o timeout.*`)}},
				MaxAttempts:     5,
			},
		},
	}

	return opts
}

// TestRun_RetryParseErrorsCoversVersionConstraintParse pins the single-unit
// half of issue #6755 for the first parse a plain `terragrunt plan` performs,
// the version-constraint partial parse: with the retry-parse-errors experiment
// on, its transient failure is retried instead of failing the command.
func TestRun_RetryParseErrorsCoversVersionConstraintParse(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsUnit(t, func(call int32) bool { return call == 1 })

	opts := singleRunOpts(t)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	err := runcmd.Run(t.Context(), thlogger.CreateLogger(), opts, v)
	require.NoError(t, err, "the transient decrypt failure is retried away")
	assert.GreaterOrEqual(t, calls.Load(), int32(2), "the failed decrypt is retried")
}

// TestRun_RetryParseErrorsCoversFullConfigParse pins the wrapper on the full
// configuration parse independently: the version-constraint parse succeeds,
// and only the later full parse fails transiently before being retried.
func TestRun_RetryParseErrorsCoversFullConfigParse(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsUnit(t, func(call int32) bool { return call == 2 })

	opts := singleRunOpts(t)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	err := runcmd.Run(t.Context(), thlogger.CreateLogger(), opts, v)
	require.NoError(t, err, "the transient decrypt failure in the full parse is retried away")
	assert.GreaterOrEqual(t, calls.Load(), int32(3), "the failed decrypt is retried")
}

// TestRun_SingleUnitParseFailsFastWithoutExperiment pins that without the
// experiment the first transient parse failure still fails the command.
func TestRun_SingleUnitParseFailsFastWithoutExperiment(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsUnit(t, func(call int32) bool { return call == 1 })

	opts := singleRunOpts(t)

	err := runcmd.Run(t.Context(), thlogger.CreateLogger(), opts, v)
	require.Error(t, err, "without the experiment the parse failure surfaces")
	require.ErrorContains(t, err, "i/o timeout", "the original transient error is what surfaces")
	assert.EqualValues(t, 1, calls.Load(), "the first parse is not retried")
}

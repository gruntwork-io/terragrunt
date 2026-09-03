package graph_test

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/errorconfig"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/runner/graph"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vsops"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const memGraphRoot = "/repo"

// flakySopsGraphVenv builds an in-memory venv holding one unit whose locals
// decrypt a secret, with a SOPS decrypter failing the first failures calls
// with a transient network error before succeeding.
func flakySopsGraphVenv(t *testing.T, failures int) (*venv.Venv, *atomic.Int32) {
	t.Helper()

	var calls atomic.Int32

	v := venvtest.New().
		WithHandler(func(context.Context, vexec.Invocation) vexec.Result {
			return vexec.Result{Stdout: []byte("OpenTofu v1.9.0\n")}
		}).
		WithSops(vsops.NewMemDecrypter(func(map[string]string, string, string) ([]byte, error) {
			if int(calls.Add(1)) <= failures {
				return nil, errors.New("dial tcp 1.2.3.4:443: i/o timeout")
			}

			return []byte(`{"value":"ok"}`), nil
		}))

	unitDir := filepath.Join(memGraphRoot, "app")
	require.NoError(t, v.FS.MkdirAll(unitDir, 0o755))
	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(unitDir, "terragrunt.hcl"),
		[]byte("locals {\n  secret = sops_decrypt_file(\"secret.enc.json\")\n}\n"),
		0o644,
	))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(unitDir, "main.tf"), []byte(""), 0o644))

	return v, &calls
}

// graphRunOpts returns options for `run --graph plan` rooted at the unit, with
// a zero-sleep retry rule matching "i/o timeout".
func graphRunOpts(t *testing.T) *options.TerragruntOptions {
	t.Helper()

	unitDir := filepath.Join(memGraphRoot, "app")

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(unitDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = unitDir
	opts.RootWorkingDir = unitDir
	opts.GraphRoot = memGraphRoot
	opts.TerraformCommand = tf.CommandNamePlan
	opts.TerraformCliArgs = iacargs.New(tf.CommandNamePlan)
	opts.SummaryDisable = true
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

// TestGraphRun_RetryParseErrorsCoversRootParse pins the run --graph half of
// issue #6755: with the retry-parse-errors experiment on, a transient failure
// in the root configuration parse is retried instead of failing the command.
func TestGraphRun_RetryParseErrorsCoversRootParse(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsGraphVenv(t, 1)

	opts := graphRunOpts(t)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.RetryParseErrors))

	err := graph.Run(t.Context(), thlogger.CreateLogger(), v, opts)
	require.NoError(t, err, "the transient decrypt failure is retried away")
	assert.GreaterOrEqual(t, calls.Load(), int32(2), "the failed decrypt is retried")
}

// TestGraphRun_ParseErrorsFailFastWithoutExperiment pins that without the
// experiment the first transient parse failure still fails run --graph.
func TestGraphRun_ParseErrorsFailFastWithoutExperiment(t *testing.T) {
	t.Parallel()

	v, calls := flakySopsGraphVenv(t, 1)

	opts := graphRunOpts(t)

	err := graph.Run(t.Context(), thlogger.CreateLogger(), v, opts)
	require.Error(t, err, "without the experiment the parse failure surfaces")
	require.ErrorContains(t, err, "i/o timeout", "the original transient error is what surfaces")
	// One parse attempt evaluates locals at most twice (partial + full parse); more would mean a retry.
	assert.LessOrEqual(t, calls.Load(), int32(2), "the root parse is not retried")
}

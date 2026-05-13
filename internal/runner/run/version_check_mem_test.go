package run_test

import (
	"context"
	"io"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newVersionTFOptions returns a minimal *tf.TFOptions wired with the mem
// backend so GetTFVersion can dispatch `terraform -version` without spawning
// a real subprocess.
func newVersionTFOptions(tfPath string, env map[string]string) *tf.TFOptions {
	return &tf.TFOptions{
		TerraformCliArgs: iacargs.New(),
		ShellOptions: shell.NewShellOptions().
			WithTFPath(tfPath).
			WithEnv(env).
			WithWriters(writer.Writers{Writer: io.Discard, ErrWriter: io.Discard}),
	}
}

func TestGetTFVersionOpenTofu(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "tofu", inv.Name)
		assert.Equal(t, []string{tf.FlagNameVersion}, inv.Args)

		return vexec.Result{Stdout: []byte("OpenTofu v1.7.2\non darwin_arm64\n")}
	})

	_, ver, impl, err := run.GetTFVersion(t.Context(), logger.CreateLogger(), exec, newVersionTFOptions("tofu", nil))
	require.NoError(t, err)
	assert.Equal(t, tfimpl.OpenTofu, impl)
	assert.Equal(t, "1.7.2", ver.String())
}

func TestGetTFVersionTerraform(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("Terraform v1.5.7\non linux_amd64\n")}
	})

	_, ver, impl, err := run.GetTFVersion(t.Context(), logger.CreateLogger(), exec, newVersionTFOptions("terraform", nil))
	require.NoError(t, err)
	assert.Equal(t, tfimpl.Terraform, impl)
	assert.Equal(t, "1.5.7", ver.String())
}

// TestGetTFVersionUnknownImplFallsBackToTerraform pins the
// "fallback to terraform when impl line is unrecognized" branch in
// GetTFVersion. The implementation is required to never surface
// tfimpl.Unknown to callers.
func TestGetTFVersionUnknownImplFallsBackToTerraform(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("Custom-Fork v0.42.0\n")}
	})

	_, ver, impl, err := run.GetTFVersion(t.Context(), logger.CreateLogger(), exec, newVersionTFOptions("custom-fork", nil))
	require.NoError(t, err)
	assert.Equal(t, tfimpl.Terraform, impl, "unknown impl must fall back to Terraform, not surface Unknown")
	assert.Equal(t, "0.42.0", ver.String())
}

func TestGetTFVersionInvalidOutput(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("not a version line\n")}
	})

	_, _, _, err := run.GetTFVersion(t.Context(), logger.CreateLogger(), exec, newVersionTFOptions("tofu", nil))
	require.Error(t, err)
}

func TestGetTFVersionPropagatesExecError(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 1, Stderr: []byte("binary missing\n")}
	})

	_, _, _, err := run.GetTFVersion(t.Context(), logger.CreateLogger(), exec, newVersionTFOptions("tofu", nil))
	require.Error(t, err)
}

// TestGetTFVersionStripsTFCLIArgs pins the contract that TF_CLI_ARGS* env
// vars are removed from the spawned environment, so user-configured
// arguments like `TF_CLI_ARGS_plan=-refresh=false` cannot interfere with
// the version probe.
func TestGetTFVersionStripsTFCLIArgs(t *testing.T) {
	t.Parallel()

	var observed atomic.Value // []string

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		observed.Store(append([]string(nil), inv.Env...))
		return vexec.Result{Stdout: []byte("OpenTofu v1.7.2\n")}
	})

	env := map[string]string{
		"PATH":             "/usr/bin",
		"TF_CLI_ARGS":      "-no-color",
		"TF_CLI_ARGS_plan": "-refresh=false",
	}

	_, _, _, err := run.GetTFVersion(t.Context(), logger.CreateLogger(), exec, newVersionTFOptions("tofu", env))
	require.NoError(t, err)

	got, _ := observed.Load().([]string)
	for _, kv := range got {
		assert.NotContains(t, kv, "TF_CLI_ARGS", "TF_CLI_ARGS* must be stripped before invoking the version probe; saw %q", kv)
	}
}

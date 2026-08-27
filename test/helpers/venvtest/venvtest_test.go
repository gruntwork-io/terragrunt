package venvtest_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewFailsClosedOnSpawn pins the default executor. A handler returning an
// empty result would report success with no output, and code that reads a
// command's stdout takes that empty string for the command's answer: an empty
// git top-level directory, an empty version, an empty list of workspaces.
func TestNewFailsClosedOnSpawn(t *testing.T) {
	t.Parallel()

	err := venvtest.New().Exec.Command(t.Context(), "git", "rev-parse", "--show-toplevel").Run()
	require.ErrorIs(t, err, vexec.ErrNoSpawn)
}

func TestNewKeepsEnvironmentMapsIndependent(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	v.Env["SDK_VISIBLE"] = "effective"

	assert.Empty(t, v.ProcessEnv)
}

// TestWithHandlerOverridesFailClosed pins the escape hatch a test uses when
// its subject is meant to run a command.
func TestWithHandlerOverridesFailClosed(t *testing.T) {
	t.Parallel()

	v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(inv.Name + " ran\n")}
	})

	out, err := v.Exec.Command(t.Context(), "tofu", "version").Output()
	require.NoError(t, err)
	assert.Equal(t, "tofu ran\n", string(out))
}

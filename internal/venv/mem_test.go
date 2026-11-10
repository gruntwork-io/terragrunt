package venv_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemVenvFailsClosedOnSpawn pins the default executor. A handler returning
// an empty result would report success with no output, and code that reads a
// command's stdout takes that empty string for the command's answer: an empty
// git top-level directory, an empty version, an empty list of workspaces.
func TestMemVenvFailsClosedOnSpawn(t *testing.T) {
	t.Parallel()

	err := venv.MemVenv().Exec.Command(t.Context(), "git", "rev-parse", "--show-toplevel").Run()
	require.ErrorIs(t, err, vexec.ErrNoSpawn)
}

// TestMemVenvWithHandlerOverridesFailClosed pins the escape hatch a test uses
// when its subject is meant to run a command.
func TestMemVenvWithHandlerOverridesFailClosed(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(inv.Name + " ran\n")}
	})

	out, err := v.Exec.Command(t.Context(), "tofu", "version").Output()
	require.NoError(t, err)
	assert.Equal(t, "tofu ran\n", string(out))
}

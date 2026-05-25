package shell_test

import (
	"bytes"
	"context"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunCommandMemBackendWithRacing verifies that passing a mem-backed
// vexec.Exec into RunCommand intercepts every subprocess invocation so neither
// tofu, terraform, nor any other binary is actually forked.
func TestRunCommandMemBackendWithRacing(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	e := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		calls.Add(1)

		switch {
		case inv.Name == "tofu" && len(inv.Args) > 0 && inv.Args[0] == "plan":
			return vexec.Result{Stdout: []byte("Plan: 0 to add\n")}
		case inv.Name == "terraform" && len(inv.Args) > 0 && inv.Args[0] == "apply":
			return vexec.Result{ExitCode: 1, Stderr: []byte("boom\n")}
		}

		return vexec.Result{ExitCode: 127, Stderr: []byte("unknown\n")}
	})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	opts := shell.NewShellOptions().
		WithWriters(writer.Writers{Writer: stdout, ErrWriter: stderr})

	l := logger.CreateLogger()

	require.NoError(t, shell.RunCommand(t.Context(), l, e, opts, "tofu", "plan"))
	assert.Contains(t, stdout.String(), "Plan: 0 to add")

	stdout.Reset()
	stderr.Reset()

	err := shell.RunCommand(t.Context(), l, e, opts, "terraform", "apply")
	require.Error(t, err)
	assert.Contains(t, stderr.String(), "boom")

	assert.Equal(t, int32(2), calls.Load(), "expected exactly two intercepted invocations")
}

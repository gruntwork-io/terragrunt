package helpers_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecRecorderCommands(t *testing.T) {
	t.Parallel()

	var invocations []vexec.Invocation

	rec := helpers.NewExecRecorder(vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		invocations = append(invocations, inv)

		return vexec.Result{Stdout: []byte("ok")}
	}))

	assert.Empty(t, rec.Commands())

	args := []string{"init", "-upgrade"}

	initCmd := rec.Command(t.Context(), "tofu", args...)
	initCmd.SetDir("/work/unit")

	out, err := initCmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "ok", string(out))

	require.NoError(t, rec.Command(t.Context(), "tofu", "version").Run())

	// The recorder keeps its own copy of the arguments.
	args[0] = "apply"

	commands := rec.Commands()
	assert.Equal(t, []helpers.RecordedCommand{
		{Name: "tofu", Args: []string{"init", "-upgrade"}, Dir: "/work/unit"},
		{Name: "tofu", Args: []string{"version"}},
	}, commands)

	// Commands hands back copies, so changing one leaves the record alone.
	commands[0].Dir = "/elsewhere"

	assert.Equal(t, "/work/unit", rec.Commands()[0].Dir)

	// Every command still runs through the wrapped Exec, in its directory.
	require.Len(t, invocations, 2)
	assert.Equal(t, "/work/unit", invocations[0].Dir)
	assert.Equal(t, "version", invocations[1].Args[0])
}

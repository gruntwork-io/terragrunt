package cli_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandsThatSpawnNoProcess pins the commands that finish without running the OpenTofu/Terraform binary, the
// version probe of provider caching included.
func TestCommandsThatSpawnNoProcess(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args []string
	}{
		{name: "find", args: []string{"find"}},
		{name: "list", args: []string{"list"}},
		{name: "dag graph", args: []string{"dag", "graph"}},
		{name: "hcl fmt", args: []string{"hcl", "fmt", "--check"}},
		{name: "hcl validate", args: []string{"hcl", "validate"}},
		{name: "info strict", args: []string{"info", "strict", "list"}},
		{name: "stack clean", args: []string{"stack", "clean"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, spawned := countingVenv(t)

			_, err := runCLI(t, v, append(tc.args, "--working-dir", unitRoot)...)
			require.NoError(t, err)

			assert.Zero(t, spawned.Load())
		})
	}
}

// TestRenderProbesTheBinaryForProviderCaching pins that a command which can read dependency outputs still sets up
// provider caching, which probes the binary's version.
func TestRenderProbesTheBinaryForProviderCaching(t *testing.T) {
	t.Parallel()

	v, spawned := countingVenv(t)

	_, err := runCLI(t, v, "render", "--working-dir", unitRoot)
	require.NoError(t, err)

	assert.Equal(t, int32(1), spawned.Load())
}

// countingVenv returns a venv holding a single unit, whose exec answers every process as OpenTofu's version command
// would, and the number of processes it has been asked to run.
func countingVenv(t *testing.T) (*venv.Venv, *atomic.Int32) {
	t.Helper()

	spawned := new(atomic.Int32)

	v := venv.MemVenv().
		WithFS(venvtest.NewFS(t, unitRoot, map[string]string{"terragrunt.hcl": ""})).
		WithHandler(func(context.Context, vexec.Invocation) vexec.Result {
			spawned.Add(1)

			return vexec.Result{Stdout: []byte("OpenTofu v1.10.0\n")}
		})

	return v, spawned
}

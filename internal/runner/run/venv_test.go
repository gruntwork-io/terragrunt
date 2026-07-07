package run_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVenvToRootRoundTrips(t *testing.T) {
	t.Parallel()

	root := venv.OSVenv()

	got := run.FromRoot(root).ToRoot()

	assert.Equal(t, root.Exec, got.Exec)
	assert.Equal(t, root.FS, got.FS)
	assert.Equal(t, root.Env, got.Env)
	assert.Equal(t, root.Writers, got.Writers)
}

func TestVenvRequireEnvPanicsWhenNil(t *testing.T) {
	t.Parallel()

	v := run.FromRoot(venv.Venv{})

	require.PanicsWithValue(t, venv.ErrVenvEnvUnset, v.RequireEnv)
}

func TestVenvRequireEnvNoPanicWhenSet(t *testing.T) {
	t.Parallel()

	v := run.FromRoot(venv.Venv{Env: map[string]string{}})

	require.NotPanics(t, v.RequireEnv)
}

func TestVenvWithWriter(t *testing.T) {
	t.Parallel()

	var original, w bytes.Buffer

	orig := run.FromRoot(venv.Venv{Writers: writer.Writers{Writer: &original, ErrWriter: &original}})

	got := orig.WithWriter(&w)

	assert.Same(t, &w, got.Writers.Writer)
	assert.Same(t, &original, orig.Writers.Writer)
}

func TestVenvWithErrWriter(t *testing.T) {
	t.Parallel()

	var original, w bytes.Buffer

	orig := run.FromRoot(venv.Venv{Writers: writer.Writers{Writer: &original, ErrWriter: &original}})

	got := orig.WithErrWriter(&w)

	assert.Same(t, &w, got.Writers.ErrWriter)
	assert.Same(t, &original, orig.Writers.ErrWriter)
}

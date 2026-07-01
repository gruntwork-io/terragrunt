package tflint_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tflint"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromRootToRootRoundTrip(t *testing.T) {
	t.Parallel()

	exec := vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{}
	})
	fs := vfs.NewMemMapFS()
	env := map[string]string{"TF_VAR_region": "us-east-1"}
	writers := writer.Writers{Writer: &bytes.Buffer{}, ErrWriter: &bytes.Buffer{}}

	root := venv.Venv{Exec: exec, FS: fs, Env: env, Writers: writers}

	local := tflint.FromRoot(root)
	require.NotNil(t, local.Exec)
	require.NotNil(t, local.FS)
	assert.Equal(t, env, local.Env)
	assert.Equal(t, writers, local.Writers)

	back := local.ToRoot()
	assert.Equal(t, root.Exec, back.Exec)
	assert.Equal(t, root.FS, back.FS)
	assert.Equal(t, root.Env, back.Env)
	assert.Equal(t, root.Writers, back.Writers)
}

func TestOSVenv(t *testing.T) {
	t.Parallel()

	v := tflint.OSVenv()
	assert.NotNil(t, v.Exec)
	assert.NotNil(t, v.FS)
}

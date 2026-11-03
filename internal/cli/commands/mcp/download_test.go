package mcp_test

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitSourceToken is the credential in [unitGitSourceConfig]'s source address.
const gitSourceToken = "s3cr3t-token"

// unitGitSourceConfig points a unit at a git source, so producing its outputs
// starts with a git download.
const unitGitSourceConfig = `
terraform {
  source = "git::https://user:` + gitSourceToken + `@example.invalid/org/modules.git//vpc?ref=v1.0.0"
}
`

// TestRenderConfigRefusesGitDownloadsWithoutExec pins that a dependency whose
// outputs need its git source downloaded does not run git when the server was
// not started with --allow=exec. The download is refused and reported without
// the source's credentials, and the render falls back to the dependency's
// mock_outputs.
func TestRenderConfigRefusesGitDownloadsWithoutExec(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"a/terragrunt.hcl": unitGitSourceConfig,
		"b/terragrunt.hcl": unitBConfig,
	})

	session := newTestSession(t, dir)

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "b"}, &out)

	assert.True(t, slices.ContainsFunc(out.Degraded, func(note string) bool {
		return strings.HasPrefix(note, `denied "git clone -- https://example.invalid/org/modules.git `) &&
			strings.HasSuffix(note, `": sources that need it were not downloaded`)
	}), "no note reports the refused clone: %v", out.Degraded)

	for _, note := range out.Degraded {
		assert.NotContains(t, note, gitSourceToken)
	}

	assert.Equal(t, []string{"a_id"}, out.Summary.InputKeys)

	var downloaded []string

	err := vfs.WalkDir(vfs.NewOSFS(), filepath.Join(dir, "a"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && d.Name() != "terragrunt.hcl" {
			downloaded = append(downloaded, path)
		}

		return nil
	})
	require.NoError(t, err)
	assert.Empty(t, downloaded, "nothing may be downloaded for the dependency")
}

package mcp_test

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gitSourceToken is the credential in [unitGitSourceConfig]'s source address.
const gitSourceToken = "fake-token"

// unitGitSourceConfig points a unit at a git source, so producing its outputs
// starts with a git download.
const unitGitSourceConfig = `
terraform {
  source = "git::https://user:` + gitSourceToken + `@example.invalid/org/modules.git//vpc?ref=v1.0.0"
}
`

// unitRegistrySourceConfig points a unit at a registry source, so producing
// its outputs means asking the registry where the module is.
const unitRegistrySourceConfig = `
terraform {
  source = "tfr://registry.example.invalid/org/vpc/aws?version=1.0.0"
}
`

// TestRenderConfigSkipsGitDownloadsWithoutExec pins that a dependency whose
// outputs need its git source downloaded starts no process when the server
// was not started with --allow=exec, even where it may write the download.
// The render reports the refused tofu without the source's credentials and
// falls back to the dependency's mock_outputs.
func TestRenderConfigSkipsGitDownloadsWithoutExec(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"a/terragrunt.hcl": unitGitSourceConfig,
		"b/terragrunt.hcl": unitBConfig,
	})

	var ran atomic.Int32

	v := venvtest.NewWithOSFS().WithHandler(func(context.Context, vexec.Invocation) vexec.Result {
		ran.Add(1)

		return vexec.Result{}
	})

	session := newTestSessionWith(t, dir, v, allowFSRW, nil)

	var out renderConfigOutput

	callTool(t, session, "render_config", map[string]any{"working_dir": "b"}, &out)

	assert.Equal(
		t,
		[]string{`denied "tofu -version": dependency outputs degraded to mock_outputs where configured`},
		out.Degraded,
	)
	assert.Zero(t, ran.Load(), "no process may start without exec")

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

// TestRenderConfigRefusesRegistryDownloadsWithoutFSRW pins that a dependency
// whose outputs need its registry source downloaded never reaches the registry
// when the server was not started with --allow=fs-rw.
func TestRenderConfigRefusesRegistryDownloadsWithoutFSRW(t *testing.T) {
	t.Parallel()

	for _, allow := range [][]string{{"http"}, {"exec", "http"}} {
		t.Run(strings.Join(allow, "+"), func(t *testing.T) {
			t.Parallel()

			dir := writeTree(t, map[string]string{
				"a/terragrunt.hcl": unitRegistrySourceConfig,
				"b/terragrunt.hcl": unitBConfig,
			})

			var requests atomic.Int32

			v := venvtest.NewWithOSFS()
			v.HTTP = vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
				requests.Add(1)

				return vhttp.Respond(http.StatusNotFound, nil, nil), nil
			})

			session := newTestSessionWith(t, dir, v, func(o *tgmcp.Options) { o.Allow = allow }, nil)

			var out renderConfigOutput

			callTool(t, session, "render_config", map[string]any{"working_dir": "b"}, &out)

			assert.Equal(t, []string{"a_id"}, out.Summary.InputKeys)
			assert.NotEmpty(t, out.Degraded, "the refused tofu must be reported")
			assert.Zero(t, requests.Load(), "the registry must not be asked for a module")

			var names []string

			require.NoError(t, filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
				names = append(names, strings.TrimPrefix(path, dir))

				return err
			}))

			slices.Sort(names)

			assert.Equal(t, []string{
				"",
				filepath.FromSlash("/a"),
				filepath.FromSlash("/a/terragrunt.hcl"),
				filepath.FromSlash("/b"),
				filepath.FromSlash("/b/terragrunt.hcl"),
			}, names)
		})
	}
}

package catalog_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestRunOnEmptyWorkspaceWritesNothing pins that a workspace with no
// configuration yields an empty catalog rather than an error: every line of a
// non-interactive run is a component, so nothing at all may be written when
// discovery finds no source.
func TestRunOnEmptyWorkspaceWritesNothing(t *testing.T) {
	t.Parallel()

	var buf strings.Builder

	v := venvtest.New().WithWriter(&buf)
	workDir := "/catalog-empty"

	require.NoError(t, v.FS.MkdirAll(workDir, 0o755))

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v, newOptions(t, workDir, catalog.FormatJSONL), "",
	)

	require.NoError(t, err)
	assert.Empty(t, buf.String(), "an empty workspace must not write a header or a summary")
}

// TestRunReportsEveryUnreachableSource pins that a run whose sources all fail
// to load reports each of them once. Discovery feeds every terraform.source it
// finds to the loader, and a repo named by two units is a clone the user would
// pay for twice.
func TestRunReportsEveryUnreachableSource(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		sources       map[string]string
		wantURLs      []string
		wantAttempted int
	}{
		{
			name: "one source per unit",
			sources: map[string]string{
				"vpc": "github.com/acme/vpc//modules/x",
				"ecs": "github.com/acme/ecs//modules/x",
			},
			wantURLs:      []string{"github.com/acme/ecs", "github.com/acme/vpc"},
			wantAttempted: 2,
		},
		{
			name: "two units of the same repo",
			sources: map[string]string{
				"vpc-app": "github.com/acme/vpc//modules/app",
				"vpc-net": "github.com/acme/vpc//modules/net",
			},
			wantURLs:      []string{"github.com/acme/vpc"},
			wantAttempted: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf strings.Builder

			v := venvtest.New().WithWriter(&buf)
			workDir := "/catalog-sources"

			for unit, source := range tc.sources {
				writeUnit(t, v, filepath.Join(workDir, unit), source)
			}

			err := catalog.Run(
				t.Context(), logger.CreateLogger(), v, newOptions(t, workDir, catalog.FormatJSONL), "",
			)

			var loadErr *tui.SourceLoadError

			require.ErrorAs(t, err, &loadErr)
			require.ErrorIs(t, err, module.ErrRemoteCloneFSNotOS,
				"the per-source cause must stay reachable through the aggregate error")
			assert.Equal(t, tc.wantAttempted, loadErr.Attempted)
			assert.Equal(t, tc.wantURLs, failedURLs(loadErr))
			assert.True(t, loadErr.AllFailed())
			assert.Empty(t, buf.String())
		})
	}
}

// TestRunLoadsARepoNamedTwiceOnce pins that a repo reaching the loader twice
// is loaded once. The catalog block forwards its URLs unfiltered, so a repo
// listed twice is a clone the user would pay for twice.
func TestRunLoadsARepoNamedTwiceOnce(t *testing.T) {
	t.Parallel()

	// The root config holding the catalog block is found by walking the real filesystem.
	rootDir := t.TempDir()

	// A local path that does not exist fails in the getter without reaching the network.
	repoURL := filepath.Join(rootDir, "missing-repo")

	v := venvtest.NewWithOSFS()

	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(rootDir, "root.hcl"),
		[]byte("catalog {\n  urls = [\""+repoURL+"\", \""+repoURL+"\"]\n}\n"),
		0o644,
	))

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v, newOptions(t, rootDir, catalog.FormatJSONL), "",
	)

	var loadErr *tui.SourceLoadError

	require.ErrorAs(t, err, &loadErr)
	assert.Equal(t, []string{repoURL}, failedURLs(loadErr), "the repo must be loaded once, not once per discoverer")
	assert.Equal(t, 1, loadErr.Attempted)
}

// TestRunWritesComponentsFromTheCatalogBlock pins that the catalog block of
// the root config is honored by the non-interactive formats, and that each
// component of the source it names is written.
func TestRunWritesComponentsFromTheCatalogBlock(t *testing.T) {
	t.Parallel()

	// The root config is found by walking the real filesystem.
	rootDir := t.TempDir()
	repoDir := filepath.Join(rootDir, "repo")

	var buf strings.Builder

	v := venvtest.NewWithOSFS().WithWriter(&buf)

	writeLocalRepo(t, v, repoDir)
	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(rootDir, "root.hcl"),
		[]byte("catalog {\n  urls = [\""+repoDir+"\"]\n}\n"),
		0o644,
	))

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v, newOptions(t, rootDir, catalog.FormatJSONL), "",
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "bravo"}, sortedDirs(t, buf.String()))
}

// TestRunToleratesAnUnparseableCatalogConfig pins that a root config the
// parser chokes on ends the run cleanly with no components instead of failing
// it: a config a user is midway through editing must not break the command.
func TestRunToleratesAnUnparseableCatalogConfig(t *testing.T) {
	t.Parallel()

	// The root config is found by walking the real filesystem.
	rootDir := t.TempDir()

	var buf strings.Builder

	v := venvtest.NewWithOSFS().WithWriter(&buf)

	require.NoError(t, vfs.WriteFile(
		v.FS, filepath.Join(rootDir, "root.hcl"), []byte("catalog {\n  urls = [\n}\n"), 0o644,
	))

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v, newOptions(t, rootDir, catalog.FormatJSONL), "",
	)

	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

// TestRunWithAnExplicitRepoURLSkipsDiscovery pins that a URL given on the
// command line is the only source loaded: the failure comes back as it is,
// rather than as the aggregate discovery reports.
func TestRunWithAnExplicitRepoURLSkipsDiscovery(t *testing.T) {
	t.Parallel()

	v := venvtest.New()
	workDir := "/catalog-explicit"

	writeUnit(t, v, filepath.Join(workDir, "vpc"), "github.com/acme/discovered//modules/x")

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), v,
		newOptions(t, workDir, catalog.FormatJSONL), "github.com/acme/explicit",
	)

	require.ErrorIs(t, err, module.ErrRemoteCloneFSNotOS)
	require.NotErrorAs(t, err, new(*tui.SourceLoadError),
		"an explicit repo URL must not run discovery")
}

// TestRunRejectsAnUnknownFormat pins that an unsupported format is rejected
// before anything is cloned, so a typo cannot cost the user a repository fetch.
func TestRunRejectsAnUnknownFormat(t *testing.T) {
	t.Parallel()

	err := catalog.Run(
		t.Context(), logger.CreateLogger(), venvtest.New(),
		newOptions(t, "/catalog-unknown-format", "yaml"), "github.com/acme/vpc",
	)

	require.ErrorIs(t, err, format.ErrUnsupportedFormat)
}

// newOptions returns catalog options for a run rooted at workDir, reading its
// root configuration from a root.hcl.
func newOptions(t *testing.T, workDir, outputFormat string) *catalog.Options {
	t.Helper()

	tgOpts := options.NewTerragruntOptions()
	tgOpts.WorkingDir = workDir
	tgOpts.RootWorkingDir = workDir
	tgOpts.TerragruntConfigPath = filepath.Join(workDir, "terragrunt.hcl")
	tgOpts.ScaffoldRootFileName = "root.hcl"

	opts := catalog.NewOptions(tgOpts)
	opts.Format = outputFormat

	return opts
}

// writeUnit writes a unit that runs the given OpenTofu/Terraform source.
func writeUnit(t *testing.T, v *venv.Venv, dir, source string) {
	t.Helper()

	require.NoError(t, v.FS.MkdirAll(dir, 0o755))
	require.NoError(t, vfs.WriteFile(
		v.FS,
		filepath.Join(dir, "terragrunt.hcl"),
		[]byte("terraform {\n  source = \""+source+"\"\n}\n"),
		0o644,
	))
}

// writeLocalRepo writes a checked-out repository holding two components.
func writeLocalRepo(t *testing.T, v *venv.Venv, repoDir string) {
	t.Helper()

	require.NoError(t, v.FS.MkdirAll(filepath.Join(repoDir, ".git"), 0o755))
	require.NoError(t, vfs.WriteFile(
		v.FS, filepath.Join(repoDir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644,
	))
	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(repoDir, ".git", "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/acme/repo
`), 0o644))

	for _, name := range []string{"alpha", "bravo"} {
		dir := filepath.Join(repoDir, name)

		require.NoError(t, v.FS.MkdirAll(dir, 0o755))
		require.NoError(t, vfs.WriteFile(
			v.FS, filepath.Join(dir, "main.tf"), []byte("# "+name+"\n"), 0o644,
		))
	}
}

// failedURLs returns the reported source URLs, sorted: loads run concurrently,
// so no test may depend on the order they fail in.
func failedURLs(err *tui.SourceLoadError) []string {
	urls := make([]string, 0, len(err.Failures))

	for _, failure := range err.Failures {
		urls = append(urls, failure.URL)
	}

	slices.Sort(urls)

	return urls
}

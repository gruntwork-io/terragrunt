package test_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/cli/flags"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCatalogRedesignIgnoreFileFlagAction drives the --ignore-file flag's
// Action the same way the CLI parser would: it resolves relative paths
// against opts.WorkingDir, rejects missing paths and directories, and
// writes the resolved absolute path back to opts.CatalogIgnoreFile.
func TestCatalogRedesignIgnoreFileFlagAction(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	ignoreFile := filepath.Join(workDir, "ignore-rules")
	require.NoError(t, os.WriteFile(ignoreFile, []byte("examples\n"), 0644))

	externalDir := t.TempDir()
	externalFile := filepath.Join(externalDir, "external-rules")
	require.NoError(t, os.WriteFile(externalFile, []byte("test/**\n"), 0644))

	cases := []struct {
		name       string
		input      string
		wantResult string
		wantErr    bool
	}{
		{
			name:       "empty input is a no-op",
			input:      "",
			wantResult: "",
		},
		{
			name:       "absolute path passes through",
			input:      externalFile,
			wantResult: externalFile,
		},
		{
			name:       "relative path resolves against WorkingDir",
			input:      "ignore-rules",
			wantResult: ignoreFile,
		},
		{
			name:    "missing path is rejected",
			input:   filepath.Join(workDir, "does-not-exist"),
			wantErr: true,
		},
		{
			name:    "directory is rejected",
			input:   workDir,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest(filepath.Join(workDir, "terragrunt.hcl"))
			require.NoError(t, err)

			opts.WorkingDir = workDir

			action := ignoreFileAction(t, opts)

			err = action(t.Context(), nil, tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, opts.CatalogIgnoreFile)
		})
	}
}

// TestCatalogRedesignDiscoveryWithIgnoreFiles exercises the full discovery
// pipeline against a local fixture: whole-repo walk, module/template
// classification, repo-root .terragrunt-catalog-ignore, and a layered
// --ignore-file (with negation that re-includes a path the repo file
// would otherwise exclude).
func TestCatalogRedesignDiscoveryWithIgnoreFiles(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	writeFixtureFile(t, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# vpc module")
	writeFixtureFile(t, filepath.Join(repoDir, "templates", "service", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFixtureFile(t, filepath.Join(repoDir, "examples", "vpc", "main.tf"), "# ignored by repo file")
	writeFixtureFile(t, filepath.Join(repoDir, "integration", "vpc", "main.tf"), "# ignored by extra file")
	writeFixtureFile(t, filepath.Join(repoDir, "stash", "keep", "main.tf"), "# re-included by extra negation")
	writeFixtureFile(t, filepath.Join(repoDir, "stash", "drop", "main.tf"), "# still excluded")

	writeFixtureFile(t, filepath.Join(repoDir, ".terragrunt-catalog-ignore"),
		"examples\nexamples/**\nstash/**\n")

	extraDir := t.TempDir()
	extraIgnore := filepath.Join(extraDir, "extra-ignore")
	require.NoError(t, os.WriteFile(extraIgnore, []byte("integration/**\n!stash/keep\n"), 0644))

	seedFakeGit(t, repoDir)

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), vfs.NewOSFS(), &module.RepoOpts{
		CloneURL:       repoDir,
		Path:           repoDir,
		RootWorkingDir: repoDir,
	})
	require.NoError(t, err)

	components, err := tui.NewComponentDiscovery().
		WithExtraIgnoreFile(extraIgnore).
		Discover(repo)
	require.NoError(t, err)

	got := map[string]tui.ComponentKind{}
	for _, c := range components {
		got[c.Dir] = c.Kind
	}

	want := map[string]tui.ComponentKind{
		"modules/vpc":       tui.ComponentKindModule,
		"templates/service": tui.ComponentKindTemplate,
		"stash/keep":        tui.ComponentKindModule,
	}

	assert.Equal(t, want, got)
}

// TestCatalogRedesignNonTTYFailsFast verifies that running the redesigned
// catalog without an interactive terminal exits with the friendly typed
// error instead of bubbletea's raw TTY failure.
//
// The guard mirrors the command's own TTY probe: when the test environment
// has a controlling terminal (a developer's shell), the command would
// launch the real TUI and block, so the test only runs where a TTY is
// genuinely unavailable (e.g. CI runners).
func TestCatalogRedesignNonTTYFailsFast(t *testing.T) {
	t.Parallel()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		t.Skip("stdin is a terminal; the catalog TUI would launch for real")
	}

	if in, out, err := tea.OpenTTY(); err == nil {
		closeErr := in.Close()
		if out != in {
			closeErr = errors.Join(closeErr, out.Close())
		}

		require.NoError(t, closeErr)
		t.Skip("a controlling terminal is available; the catalog TUI would launch for real")
	}

	workDir := t.TempDir()

	_, _, err := helpers.RunTerragruntCommandWithOutput(t,
		"terragrunt catalog --working-dir "+workDir)

	require.Error(t, err)
	require.ErrorIs(t, err, tui.ErrNoTerminal)
}

func ignoreFileAction(t *testing.T, opts *options.TerragruntOptions) clihelper.FlagActionFunc[string] {
	t.Helper()

	flagList := catalog.NewFlags(opts, nil)

	flag := flagList.Get(catalog.IgnoreFileFlagName)
	require.NotNil(t, flag, "--%s flag not registered", catalog.IgnoreFileFlagName)

	wrapper, ok := flag.(*flags.Flag)
	require.True(t, ok, "expected *flags.Flag wrapper, got %T", flag)

	inner, ok := wrapper.Flag.(*clihelper.GenericFlag[string])
	require.True(t, ok, "expected *clihelper.GenericFlag[string], got %T", wrapper.Flag)

	require.NotNil(t, inner.Action, "--%s flag is missing its Action", catalog.IgnoreFileFlagName)

	return inner.Action
}

func writeFixtureFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func seedFakeGit(t *testing.T, repoDir string) {
	t.Helper()

	gitDir := filepath.Join(repoDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/gruntwork-io/fake-repo
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644))
}

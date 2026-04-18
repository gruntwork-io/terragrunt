package redesign_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newFakeRepo creates a bare-minimum cloned repo on disk that module.NewRepo
// can successfully consume (requires .git/config and .git/HEAD). The returned
// *module.Repo has the walk-relevant Path() / CloneURL() pointing at repoDir.
func newFakeRepo(t *testing.T, repoDir string) *module.Repo {
	t.Helper()

	gitDir := filepath.Join(repoDir, ".git")
	require.NoError(t, os.MkdirAll(gitDir, 0755))

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/gruntwork-io/fake-repo
`), 0644))

	require.NoError(t, os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0644))

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), module.RepoOpts{
		CloneURL:       repoDir,
		Path:           repoDir,
		RootWorkingDir: repoDir,
	})
	require.NoError(t, err)

	return repo
}

// writeFile is a shorthand that ensures parent directories exist.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

// TestDiscoverComponents_ClassifiesFixtureTree asserts that the walker
// correctly classifies each known shape in a representative repo layout.
func TestDiscoverComponents_ClassifiesFixtureTree(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	// foo/ is a plain module (has main.tf, no boilerplate).
	writeFile(t, filepath.Join(repoDir, "foo", "main.tf"), "# vpc terraform")

	// bar/ has a .boilerplate/ subdir — template at bar/.
	writeFile(t, filepath.Join(repoDir, "bar", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFile(t, filepath.Join(repoDir, "bar", ".boilerplate", "README.md"), "# bar template boilerplate dir")

	// baz/ has a top-level boilerplate.yml — template at baz/.
	writeFile(t, filepath.Join(repoDir, "baz", "boilerplate.yml"), "variables: []\n")

	// qux/ has both main.tf AND a .boilerplate/ — template wins.
	writeFile(t, filepath.Join(repoDir, "qux", "main.tf"), "# mixed")
	writeFile(t, filepath.Join(repoDir, "qux", ".boilerplate", "boilerplate.yml"), "variables: []\n")

	// Nested boilerplate.yml inside bar/.boilerplate/ must NOT surface as
	// a separate template (bar/ already SkipDir'd the subtree).
	writeFile(t, filepath.Join(repoDir, "bar", ".boilerplate", "nested", "boilerplate.yml"), "variables: []\n")

	// Hidden dirs at top level must be skipped entirely.
	writeFile(t, filepath.Join(repoDir, ".terraform", "main.tf"), "# should be skipped")

	// A nested module (modules/foo/main.tf) to ensure we still walk into
	// non-boilerplate subdirs of modules that contain tf files.
	writeFile(t, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# nested module")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.DiscoverComponents(repo, false)
	require.NoError(t, err)

	got := map[string]redesign.ComponentKind{}
	for _, c := range components {
		got[c.Dir] = c.Kind
	}

	want := map[string]redesign.ComponentKind{
		"foo":         redesign.ComponentKindModule,
		"bar":         redesign.ComponentKindTemplate,
		"baz":         redesign.ComponentKindTemplate,
		"qux":         redesign.ComponentKindTemplate,
		"modules/vpc": redesign.ComponentKindModule,
	}

	assert.Equal(t, want, got, "unexpected component classification")

	// Sanity-check the .boilerplate subtree was skipped.
	for dir := range got {
		assert.NotContains(t, dir, ".boilerplate", "no component should be derived from a .boilerplate subtree: %s", dir)
	}
}

// TestDiscoverComponents_RepoRootAsComponent asserts the repo root itself
// is classified when it qualifies (e.g. a repo that IS a template).
func TestDiscoverComponents_RepoRootAsComponent(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	// Repo root has a .boilerplate dir → root is a template.
	writeFile(t, filepath.Join(repoDir, ".boilerplate", "boilerplate.yml"), "variables: []\n")

	// A child module that must NOT surface: once the root is classified as
	// a template, the walker SkipDirs the whole tree.
	writeFile(t, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# should be skipped")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.DiscoverComponents(repo, false)
	require.NoError(t, err)

	require.Len(t, components, 1, "only the root template should surface")
	assert.Equal(t, redesign.ComponentKindTemplate, components[0].Kind)
	assert.Empty(t, components[0].Dir, "root component should have an empty Dir")
}

// TestDiscoverComponents_EmptyRepo returns no components for an empty tree.
func TestDiscoverComponents_EmptyRepo(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)
	repo := newFakeRepo(t, repoDir)

	components, err := redesign.DiscoverComponents(repo, false)
	require.NoError(t, err)
	assert.Empty(t, components)
}

// TestComponent_TerraformSourcePath covers both root and subdirectory forms.
func TestComponent_TerraformSourcePath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		cloneURL string
		dir      string
		want     string
	}{
		{
			name:     "root component",
			cloneURL: "github.com/org/repo",
			dir:      "",
			want:     "github.com/org/repo",
		},
		{
			name:     "subdir component",
			cloneURL: "github.com/org/repo",
			dir:      "modules/vpc",
			want:     "github.com/org/repo//modules/vpc",
		},
		{
			name:     "preserves query after subdir",
			cloneURL: "github.com/org/repo?ref=v1.0.0",
			dir:      "modules/vpc",
			want:     "github.com/org/repo//modules/vpc?ref=v1.0.0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := redesign.NewComponentForTest(redesign.ComponentKindModule, tc.cloneURL, tc.dir, "")
			assert.Equal(t, tc.want, c.TerraformSourcePath())
		})
	}
}

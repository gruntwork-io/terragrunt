package redesign_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoverComponents_WithCustomFS proves discovery runs against an
// injected vfs.FS — passing vfs.NewOSFS() explicitly produces the same
// result as the zero-arg constructor's internal default.
func TestDiscoverComponents_WithCustomFS(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)
	writeFile(t, filepath.Join(repoDir, "foo", "main.tf"), "# module")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(vfs.NewOSFS()).Discover(repo)
	require.NoError(t, err)
	require.Len(t, components, 1)
	assert.Equal(t, "foo", components[0].Dir)
	assert.Equal(t, redesign.ComponentKindModule, components[0].Kind)
}

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

	// bar/ has a .boilerplate/ subdir. Template at bar/.
	writeFile(t, filepath.Join(repoDir, "bar", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFile(t, filepath.Join(repoDir, "bar", ".boilerplate", "README.md"), "# bar template boilerplate dir")

	// baz/ has a top-level boilerplate.yml. Template at baz/.
	writeFile(t, filepath.Join(repoDir, "baz", "boilerplate.yml"), "variables: []\n")

	// qux/ has both main.tf AND a .boilerplate/. Template wins.
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

	components, err := redesign.NewComponentDiscovery().Discover(repo)
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

// TestDiscoverComponents_UnitsAndStacks asserts units and stacks are
// classified correctly, that template > stack > unit > module precedence
// holds, and that a stack/unit's subtree is not walked into.
func TestDiscoverComponents_UnitsAndStacks(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	// unit-a/ is a plain unit.
	writeFile(t, filepath.Join(repoDir, "unit-a", "terragrunt.hcl"), "# unit a")

	// stack-a/ is a plain stack.
	writeFile(t, filepath.Join(repoDir, "stack-a", "terragrunt.stack.hcl"), "# stack a")

	// mixed-unit/ has both terragrunt.hcl and main.tf → unit wins over module.
	writeFile(t, filepath.Join(repoDir, "mixed-unit", "terragrunt.hcl"), "# mixed unit")
	writeFile(t, filepath.Join(repoDir, "mixed-unit", "main.tf"), "# ignored")

	// mixed-stack/ has both terragrunt.stack.hcl and terragrunt.hcl → stack wins.
	writeFile(t, filepath.Join(repoDir, "mixed-stack", "terragrunt.stack.hcl"), "# stack")
	writeFile(t, filepath.Join(repoDir, "mixed-stack", "terragrunt.hcl"), "# also present")

	// templated-stack/ has a .boilerplate/ alongside a terragrunt.stack.hcl →
	// template wins.
	writeFile(t, filepath.Join(repoDir, "templated-stack", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFile(t, filepath.Join(repoDir, "templated-stack", "terragrunt.stack.hcl"), "# stack")

	// templated-unit/ has a .boilerplate/ alongside a terragrunt.hcl →
	// template wins.
	writeFile(t, filepath.Join(repoDir, "templated-unit", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFile(t, filepath.Join(repoDir, "templated-unit", "terragrunt.hcl"), "# unit")

	// A nested .tf file under a unit must NOT surface as a second module.
	// The unit's subtree is SkipDir'd.
	writeFile(t, filepath.Join(repoDir, "unit-a", "nested", "main.tf"), "# should not surface")

	// A nested unit under a stack must NOT surface. The stack's subtree is
	// SkipDir'd.
	writeFile(t, filepath.Join(repoDir, "stack-a", "generated", "terragrunt.hcl"), "# should not surface")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().Discover(repo)
	require.NoError(t, err)

	got := map[string]redesign.ComponentKind{}
	for _, c := range components {
		got[c.Dir] = c.Kind
	}

	want := map[string]redesign.ComponentKind{
		"unit-a":          redesign.ComponentKindUnit,
		"stack-a":         redesign.ComponentKindStack,
		"mixed-unit":      redesign.ComponentKindUnit,
		"mixed-stack":     redesign.ComponentKindStack,
		"templated-stack": redesign.ComponentKindTemplate,
		"templated-unit":  redesign.ComponentKindTemplate,
	}

	assert.Equal(t, want, got)
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

	components, err := redesign.NewComponentDiscovery().Discover(repo)
	require.NoError(t, err)

	require.Len(t, components, 1, "only the root template should surface")
	assert.Equal(t, redesign.ComponentKindTemplate, components[0].Kind)
	assert.Empty(t, components[0].Dir, "root component should have an empty Dir")
}

// TestDiscoverComponents_HonorsIgnoreFile asserts that dirs matched by
// .terragrunt-catalog-ignore are skipped, and that negation rules can
// re-include siblings of an ignored pattern (but not descendants of an
// already-skipped parent).
func TestDiscoverComponents_HonorsIgnoreFile(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	writeFile(t, filepath.Join(repoDir, "foo", "main.tf"), "# module")
	writeFile(t, filepath.Join(repoDir, "examples", "foo", "main.tf"), "# example, should be ignored")
	writeFile(t, filepath.Join(repoDir, "test", "drop", "main.tf"), "# should be ignored")
	writeFile(t, filepath.Join(repoDir, "test", "keep", "main.tf"), "# re-included via negation")

	writeFile(t, filepath.Join(repoDir, ".terragrunt-catalog-ignore"),
		"# skip examples and everything under it\nexamples\nexamples/**\ntest/**\n!test/keep\n")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().Discover(repo)
	require.NoError(t, err)

	got := map[string]redesign.ComponentKind{}
	for _, c := range components {
		got[c.Dir] = c.Kind
	}

	want := map[string]redesign.ComponentKind{
		"foo":       redesign.ComponentKindModule,
		"test/keep": redesign.ComponentKindModule,
	}

	assert.Equal(t, want, got, "ignore file should exclude examples/** and test/** except test/keep")
}

// TestDiscoverComponents_ExtraIgnoreFile asserts that an extra ignore file
// layered on top of .terragrunt-catalog-ignore extends the repo's rules and
// can re-include a path the repo file excluded via negation.
func TestDiscoverComponents_ExtraIgnoreFile(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	writeFile(t, filepath.Join(repoDir, "foo", "main.tf"), "# kept")
	writeFile(t, filepath.Join(repoDir, "examples", "foo", "main.tf"), "# repo-ignored")
	writeFile(t, filepath.Join(repoDir, "integration", "vpc", "main.tf"), "# extra-ignored")
	writeFile(t, filepath.Join(repoDir, "stash", "keep", "main.tf"), "# re-included")

	writeFile(t, filepath.Join(repoDir, ".terragrunt-catalog-ignore"),
		"examples\nexamples/**\nstash/**\n")

	extraDir := t.TempDir()
	extraPath := filepath.Join(extraDir, "extra-ignore")
	writeFile(t, extraPath, "integration/**\n!stash/keep\n")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().WithExtraIgnoreFile(extraPath).Discover(repo)
	require.NoError(t, err)

	got := map[string]redesign.ComponentKind{}
	for _, c := range components {
		got[c.Dir] = c.Kind
	}

	want := map[string]redesign.ComponentKind{
		"foo":        redesign.ComponentKindModule,
		"stash/keep": redesign.ComponentKindModule,
	}

	assert.Equal(t, want, got, "extra ignore file should extend repo rules and re-include via negation")
}

// TestDiscoverComponents_EmptyRepo returns no components for an empty tree.
func TestDiscoverComponents_EmptyRepo(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)
	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().Discover(repo)
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

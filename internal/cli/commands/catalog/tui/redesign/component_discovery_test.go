package redesign_test

import (
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

// testRepoDir and testWorkingDir are stable in-memory paths used across the
// redesign test suite. Both live under root so afero's MemMapFs can host
// the fixture trees without colliding with real OS paths.
const (
	testRepoDir    = "/repo"
	testWorkingDir = "/work"
)

// TestDiscoverComponents_WithCustomFS proves discovery runs against an
// injected vfs.FS — the same in-memory FS that materialized the fixture is
// passed to module.NewRepo (so .git/config and .git/HEAD are read from
// memory) and to ComponentDiscovery (so the walk runs against memory).
func TestDiscoverComponents_WithCustomFS(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir
	writeFileFS(t, fsys, filepath.Join(repoDir, "foo", "main.tf"), "# module")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).Discover(repo)
	require.NoError(t, err)
	require.Len(t, components, 1)
	assert.Equal(t, "foo", components[0].Dir)
	assert.Equal(t, redesign.ComponentKindModule, components[0].Kind)
}

// newFakeRepo creates a bare-minimum cloned repo on the given fsys that
// module.NewRepo can successfully consume (requires .git/config and
// .git/HEAD). The returned *module.Repo is wired to read through fsys, and
// its Path() / CloneURL() point at repoDir.
func newFakeRepo(t *testing.T, fsys vfs.FS, repoDir string) *module.Repo {
	t.Helper()

	gitDir := filepath.Join(repoDir, ".git")
	require.NoError(t, fsys.MkdirAll(gitDir, 0o755))

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(gitDir, "config"), []byte(`[core]
	repositoryformatversion = 0
[remote "origin"]
	url = github.com/gruntwork-io/fake-repo
`), 0o644))

	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644))

	repo, err := module.NewRepo(t.Context(), logger.CreateLogger(), fsys, &module.RepoOpts{
		CloneURL:       repoDir,
		Path:           repoDir,
		RootWorkingDir: repoDir,
	})
	require.NoError(t, err)

	return repo
}

// writeFileFS is a shorthand that writes content to an in-memory fsys,
// creating any missing parent directories.
func writeFileFS(t *testing.T, fsys vfs.FS, path, content string) {
	t.Helper()

	require.NoError(t, vfs.WriteFile(fsys, path, []byte(content), 0o644))
}

// TestDiscoverComponents_ClassifiesFixtureTree asserts that the walker
// correctly classifies each known shape in a representative repo layout.
func TestDiscoverComponents_ClassifiesFixtureTree(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir

	// foo/ is a plain module (has main.tf, no boilerplate).
	writeFileFS(t, fsys, filepath.Join(repoDir, "foo", "main.tf"), "# vpc terraform")

	// bar/ has a .boilerplate/ subdir. Template at bar/.
	writeFileFS(t, fsys, filepath.Join(repoDir, "bar", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFileFS(t, fsys, filepath.Join(repoDir, "bar", ".boilerplate", "README.md"), "# bar template boilerplate dir")

	// baz/ has a top-level boilerplate.yml. Template at baz/.
	writeFileFS(t, fsys, filepath.Join(repoDir, "baz", "boilerplate.yml"), "variables: []\n")

	// qux/ has both main.tf AND a .boilerplate/. Template wins.
	writeFileFS(t, fsys, filepath.Join(repoDir, "qux", "main.tf"), "# mixed")
	writeFileFS(t, fsys, filepath.Join(repoDir, "qux", ".boilerplate", "boilerplate.yml"), "variables: []\n")

	// Nested boilerplate.yml inside bar/.boilerplate/ must NOT surface as
	// a separate template (bar/ already SkipDir'd the subtree).
	writeFileFS(t, fsys, filepath.Join(repoDir, "bar", ".boilerplate", "nested", "boilerplate.yml"), "variables: []\n")

	// Hidden dirs at top level must be skipped entirely.
	writeFileFS(t, fsys, filepath.Join(repoDir, ".terraform", "main.tf"), "# should be skipped")

	// A nested module (modules/foo/main.tf) to ensure we still walk into
	// non-boilerplate subdirs of modules that contain tf files.
	writeFileFS(t, fsys, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# nested module")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).Discover(repo)
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

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir

	// unit-a/ is a plain unit.
	writeFileFS(t, fsys, filepath.Join(repoDir, "unit-a", "terragrunt.hcl"), "# unit a")

	// stack-a/ is a plain stack.
	writeFileFS(t, fsys, filepath.Join(repoDir, "stack-a", "terragrunt.stack.hcl"), "# stack a")

	// mixed-unit/ has both terragrunt.hcl and main.tf → unit wins over module.
	writeFileFS(t, fsys, filepath.Join(repoDir, "mixed-unit", "terragrunt.hcl"), "# mixed unit")
	writeFileFS(t, fsys, filepath.Join(repoDir, "mixed-unit", "main.tf"), "# ignored")

	// mixed-stack/ has both terragrunt.stack.hcl and terragrunt.hcl → stack wins.
	writeFileFS(t, fsys, filepath.Join(repoDir, "mixed-stack", "terragrunt.stack.hcl"), "# stack")
	writeFileFS(t, fsys, filepath.Join(repoDir, "mixed-stack", "terragrunt.hcl"), "# also present")

	// templated-stack/ has a .boilerplate/ alongside a terragrunt.stack.hcl →
	// template wins.
	writeFileFS(t, fsys, filepath.Join(repoDir, "templated-stack", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFileFS(t, fsys, filepath.Join(repoDir, "templated-stack", "terragrunt.stack.hcl"), "# stack")

	// templated-unit/ has a .boilerplate/ alongside a terragrunt.hcl →
	// template wins.
	writeFileFS(t, fsys, filepath.Join(repoDir, "templated-unit", ".boilerplate", "boilerplate.yml"), "variables: []\n")
	writeFileFS(t, fsys, filepath.Join(repoDir, "templated-unit", "terragrunt.hcl"), "# unit")

	// A nested .tf file under a unit must NOT surface as a second module.
	// The unit's subtree is SkipDir'd.
	writeFileFS(t, fsys, filepath.Join(repoDir, "unit-a", "nested", "main.tf"), "# should not surface")

	// A nested unit under a stack must NOT surface. The stack's subtree is
	// SkipDir'd.
	writeFileFS(t, fsys, filepath.Join(repoDir, "stack-a", "generated", "terragrunt.hcl"), "# should not surface")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).Discover(repo)
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

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir

	// Repo root has a .boilerplate dir → root is a template.
	writeFileFS(t, fsys, filepath.Join(repoDir, ".boilerplate", "boilerplate.yml"), "variables: []\n")

	// A child module that must NOT surface: once the root is classified as
	// a template, the walker SkipDirs the whole tree.
	writeFileFS(t, fsys, filepath.Join(repoDir, "modules", "vpc", "main.tf"), "# should be skipped")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).Discover(repo)
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

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir

	writeFileFS(t, fsys, filepath.Join(repoDir, "foo", "main.tf"), "# module")
	writeFileFS(t, fsys, filepath.Join(repoDir, "examples", "foo", "main.tf"), "# example, should be ignored")
	writeFileFS(t, fsys, filepath.Join(repoDir, "test", "drop", "main.tf"), "# should be ignored")
	writeFileFS(t, fsys, filepath.Join(repoDir, "test", "keep", "main.tf"), "# re-included via negation")

	writeFileFS(t, fsys, filepath.Join(repoDir, ".terragrunt-catalog-ignore"),
		"# skip examples and everything under it\nexamples\nexamples/**\ntest/**\n!test/keep\n")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).Discover(repo)
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

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir

	writeFileFS(t, fsys, filepath.Join(repoDir, "foo", "main.tf"), "# kept")
	writeFileFS(t, fsys, filepath.Join(repoDir, "examples", "foo", "main.tf"), "# repo-ignored")
	writeFileFS(t, fsys, filepath.Join(repoDir, "integration", "vpc", "main.tf"), "# extra-ignored")
	writeFileFS(t, fsys, filepath.Join(repoDir, "stash", "keep", "main.tf"), "# re-included")

	writeFileFS(t, fsys, filepath.Join(repoDir, ".terragrunt-catalog-ignore"),
		"examples\nexamples/**\nstash/**\n")

	extraPath := "/extra/extra-ignore"
	writeFileFS(t, fsys, extraPath, "integration/**\n!stash/keep\n")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).WithExtraIgnoreFile(extraPath).Discover(repo)
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

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir
	require.NoError(t, fsys.MkdirAll(repoDir, 0o755))

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := redesign.NewComponentDiscovery().WithFS(fsys).Discover(repo)
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

// TestComponentDiscovery_WithWalkWithSymlinksIsChainable verifies the
// opt-in symlink-follow builder returns the same pointer for chaining, and
// that a discovery run with the flag enabled still classifies a plain
// module correctly. This case stays on the OS filesystem because the
// symlink-following walker (util.WalkDirWithSymlinks) is OS-only.
func TestComponentDiscovery_WithWalkWithSymlinksIsChainable(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewOSFS()
	repoDir := helpers.TmpDirWOSymlinks(t)
	writeFileFS(t, fsys, filepath.Join(repoDir, "vpc", "main.tf"), "# module\n")

	repo := newFakeRepo(t, fsys, repoDir)

	cd := redesign.NewComponentDiscovery()
	chained := cd.WithWalkWithSymlinks()
	assert.Same(t, cd, chained, "WithWalkWithSymlinks should return the same builder for chaining")

	components, err := cd.Discover(repo)
	require.NoError(t, err)
	require.Len(t, components, 1)
	assert.Equal(t, redesign.ComponentKindModule, components[0].Kind)
}

// TestComponentFilterValueReturnsTitle exercises Component.FilterValue and
// ComponentEntry.FilterValue (which delegates), asserting both return the
// component's display title for the list's fuzzy-match filter.
func TestComponentFilterValueReturnsTitle(t *testing.T) {
	t.Parallel()

	c := redesign.NewComponentForTest(
		redesign.ComponentKindModule,
		"github.com/gruntwork-io/repo",
		"modules/vpc",
		"",
	)

	assert.Equal(t, c.Title(), c.FilterValue(),
		"Component.FilterValue should equal Title for list filtering")

	entry := redesign.NewComponentEntry(c)
	assert.Equal(t, c.Title(), entry.FilterValue(),
		"ComponentEntry.FilterValue should delegate to the inner Component")
}

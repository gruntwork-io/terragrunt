package git_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveSubmoduleURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		parentURL string
		url       string
		want      string
	}{
		{
			name:      "absolute https url unchanged",
			parentURL: "https://example.com/org/repo.git",
			url:       "https://example.com/other/dep.git",
			want:      "https://example.com/other/dep.git",
		},
		{
			name:      "absolute scp url unchanged",
			parentURL: "https://example.com/org/repo.git",
			url:       "git@github.com:org/dep.git",
			want:      "git@github.com:org/dep.git",
		},
		{
			name:      "sibling repository",
			parentURL: "https://example.com/org/repo.git",
			url:       "../sibling.git",
			want:      "https://example.com/org/sibling.git",
		},
		{
			name:      "two levels up",
			parentURL: "https://example.com/org/repo.git",
			url:       "../../other/dep.git",
			want:      "https://example.com/other/dep.git",
		},
		{
			name:      "dot slash appends",
			parentURL: "https://example.com/org/repo.git",
			url:       "./sub.git",
			want:      "https://example.com/org/repo.git/sub.git",
		},
		{
			name:      "parent with trailing slash",
			parentURL: "https://example.com/org/repo.git/",
			url:       "../sibling.git",
			want:      "https://example.com/org/sibling.git",
		},
		{
			name:      "scp parent sibling",
			parentURL: "git@github.com:org/repo.git",
			url:       "../sibling.git",
			want:      "git@github.com:org/sibling.git",
		},
		{
			name:      "scp parent exhausts path components",
			parentURL: "git@github.com:org/repo.git",
			url:       "../../sibling.git",
			want:      "git@github.com:sibling.git",
		},
		{
			name:      "mixed dot and dotdot segments",
			parentURL: "https://example.com/org/repo.git",
			url:       "./../sibling.git",
			want:      "https://example.com/org/sibling.git",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, git.ResolveSubmoduleURL(tt.parentURL, tt.url))
		})
	}
}

func TestParseSubmoduleConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		want   map[string]string
		name   string
		output string
	}{
		{
			name:   "empty output",
			output: "",
			want:   map[string]string{},
		},
		{
			name: "two submodules",
			output: "submodule.child.path\nmodules/child\x00" +
				"submodule.child.url\nhttps://example.com/child.git\x00" +
				"submodule.dep.path\nvendor/dep\x00" +
				"submodule.dep.url\n../dep.git\x00",
			want: map[string]string{
				"modules/child": "https://example.com/child.git",
				"vendor/dep":    "../dep.git",
			},
		},
		{
			name: "name containing dots",
			output: "submodule.a.b.path\nmodules/a.b\x00" +
				"submodule.a.b.url\nhttps://example.com/a.b.git\x00",
			want: map[string]string{
				"modules/a.b": "https://example.com/a.b.git",
			},
		},
		{
			name:   "missing url dropped",
			output: "submodule.child.path\nmodules/child\x00",
			want:   map[string]string{},
		},
		{
			name:   "missing path dropped",
			output: "submodule.child.url\nhttps://example.com/child.git\x00",
			want:   map[string]string{},
		},
		{
			name: "unrelated keys ignored",
			output: "core.bare\nfalse\x00" +
				"submodule.child.branch\nmain\x00" +
				"submodule.child.path\nmodules/child\x00" +
				"submodule.child.url\nhttps://example.com/child.git\x00",
			want: map[string]string{
				"modules/child": "https://example.com/child.git",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, git.ParseSubmoduleConfig(tt.output))
		})
	}
}

// TestGitRunner_SubmoduleURLs exercises the `git config --blob` path
// against a real repository: the .gitmodules blob committed by the test
// server is located through ls-tree and parsed by git itself.
func TestGitRunner_SubmoduleURLs(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	require.NoError(t, srv.CommitFile(t.Context(), "README.md", []byte("# repo"), "add readme"))

	const pinnedHash = "0123456789abcdef0123456789abcdef01234567"

	require.NoError(t, srv.CommitSubmodule(
		t.Context(), "modules/child", "https://example.com/child.git", pinnedHash, "add submodule",
	))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	runner = runner.WithWorkDir(helpers.TmpDirWOSymlinks(t))
	require.NoError(t, runner.Clone(ctx, url, true, 0, ""))

	tree, err := runner.LsTreeRecursive(ctx, "HEAD")
	require.NoError(t, err)

	var gitmodulesHash string

	for _, entry := range tree.Entries() {
		if entry.Path == git.GitmodulesPath {
			gitmodulesHash = entry.Hash
		}
	}

	require.NotEmpty(t, gitmodulesHash, ".gitmodules entry not found in tree")

	urls, err := runner.SubmoduleURLs(ctx, gitmodulesHash)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"modules/child": "https://example.com/child.git"}, urls)
}

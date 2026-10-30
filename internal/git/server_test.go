//go:build !windows

// Server has no exec seam, so these spawn real git, which the unit matrix only does on ubuntu/macos.

package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer(t *testing.T) {
	t.Parallel()

	t.Run("start and clone", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(
			t,
			srv.CommitFile(t.Context(), "README.md", []byte("# test repo"), "initial commit"),
		)

		url, err := srv.Start(t.Context())
		require.NoError(t, err)

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(venv.OSVenv())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)

		err = runner.Clone(t.Context(), url, false, 0, "")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(cloneDir, "README.md"))
		require.NoError(t, err)
		assert.Equal(t, "# test repo", string(data))
	})

	t.Run("ls-remote returns HEAD", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile(t.Context(), "file.txt", []byte("content"), "commit"))

		url, err := srv.Start(t.Context())
		require.NoError(t, err)

		runner, err := git.NewGitRunner(venv.OSVenv())
		require.NoError(t, err)

		results, err := runner.LsRemote(t.Context(), url, "HEAD")
		require.NoError(t, err)
		require.NotEmpty(t, results)
		assert.Regexp(t, "^[0-9a-f]{40}$", results[0].Hash)
	})

	t.Run("multiple files", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile(t.Context(), "a.txt", []byte("aaa"), "add a"))
		require.NoError(t, srv.CommitFile(t.Context(), "dir/b.txt", []byte("bbb"), "add b"))

		url, err := srv.Start(t.Context())
		require.NoError(t, err)

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(venv.OSVenv())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)

		err = runner.Clone(t.Context(), url, false, 0, "")
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(cloneDir, "a.txt"))
		require.NoError(t, err)
		assert.Equal(t, "aaa", string(data))

		data, err = os.ReadFile(filepath.Join(cloneDir, "dir", "b.txt"))
		require.NoError(t, err)
		assert.Equal(t, "bbb", string(data))
	})

	t.Run("clone bare", func(t *testing.T) {
		t.Parallel()

		srv, err := git.NewServer()
		require.NoError(t, err)
		t.Cleanup(func() { _ = srv.Close() })

		require.NoError(t, srv.CommitFile(t.Context(), "file.txt", []byte("content"), "commit"))

		url, err := srv.Start(t.Context())
		require.NoError(t, err)

		cloneDir := helpers.TmpDirWOSymlinks(t)
		runner, err := git.NewGitRunner(venv.OSVenv())
		require.NoError(t, err)

		runner = runner.WithWorkDir(cloneDir)

		err = runner.Clone(t.Context(), url, true, 0, "")
		require.NoError(t, err)

		// Bare clone has HEAD file at root
		_, err = os.Stat(filepath.Join(cloneDir, "HEAD"))
		require.NoError(t, err)
	})
}

func TestServer_CommitFiles(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	require.NoError(t, srv.CommitFiles(t.Context(), map[string][]byte{
		"a.txt":     []byte("aaa"),
		"dir/b.txt": []byte("bbb"),
	}, "add files"))

	cloneDir := cloneServer(t, srv)

	data, err := os.ReadFile(filepath.Join(cloneDir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "aaa", string(data))

	data, err = os.ReadFile(filepath.Join(cloneDir, "dir", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bbb", string(data))
}

func TestServer_CommitSymlink(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)

	require.NoError(t, srv.CommitFile(t.Context(), "target.txt", []byte("target"), "add target"))
	require.NoError(t, srv.CommitSymlink(t.Context(), "dir/link", "../target.txt", "add link"))

	target, err := os.Readlink(filepath.Join(cloneServer(t, srv), "dir", "link"))
	require.NoError(t, err)
	assert.Equal(t, "../target.txt", target)
}

func TestServer_CommitSubmodule(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newTestServer(t)

	require.NoError(t, srv.CommitEmpty(ctx, "seed"))

	head, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.CommitSubmodule(ctx, "vendor/first", "https://example.com/first.git", head, "first"))
	require.NoError(t, srv.CommitSubmodule(ctx, "vendor/second", "https://example.com/second.git", head, "second"))
	require.NoError(t, srv.CommitSubmodule(ctx, "vendor/orphan", "", head, "orphan"))

	runner := newExecRunner(t, cloneServer(t, srv))

	tree, err := runner.LsTreeRecursive(ctx, "HEAD")
	require.NoError(t, err)

	gitlinks := map[string]string{}
	gitmodulesHash := ""

	for _, entry := range tree.Entries() {
		if entry.Path == ".gitmodules" {
			gitmodulesHash = entry.Hash
		}

		if entry.Type == "commit" {
			gitlinks[entry.Path] = entry.Hash
		}
	}

	assert.Equal(t, map[string]string{
		"vendor/first":  head,
		"vendor/second": head,
		"vendor/orphan": head,
	}, gitlinks)

	urls, err := runner.SubmoduleURLs(ctx, gitmodulesHash)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"vendor/first":  "https://example.com/first.git",
		"vendor/second": "https://example.com/second.git",
	}, urls, "an unregistered gitlink should not appear in .gitmodules")
}

func TestServer_CommitEmptyChain(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newTestServer(t)

	require.NoError(t, srv.CommitEmpty(ctx, "seed"))

	seed, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.CommitEmptyChain(ctx, 3, "chain"))

	got, err := newExecRunner(t, cloneServer(t, srv)).RevParseCommit(ctx, "HEAD~3")
	require.NoError(t, err)
	assert.Equal(t, seed, got)
}

func TestServer_Tags(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newTestServer(t)

	require.NoError(t, srv.CommitEmpty(ctx, "seed"))
	require.NoError(t, srv.Tag(ctx, "v1.0.0"))
	require.NoError(t, srv.Tag(ctx, "v1.1.0"))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	runner := newExecRunner(t, helpers.TmpDirWOSymlinks(t))

	require.NoError(t, srv.DeleteTag(ctx, "v1.0.0"))
	require.NoError(t, srv.DeleteTag(ctx, "never-existed"), "deleting an absent tag is a no-op")

	head, err := srv.Head(ctx)
	require.NoError(t, err)

	tags, err := runner.LsRemoteTags(ctx, url)
	require.NoError(t, err)
	assert.Equal(t, []git.LsRemoteResult{{Hash: head, Ref: "refs/tags/v1.1.0"}}, tags)
}

func TestServer_Refs(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newTestServer(t)

	require.NoError(t, srv.CommitEmpty(ctx, "first"))

	first, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.CommitEmpty(ctx, "second"))

	second, err := srv.Head(ctx)
	require.NoError(t, err)

	require.NoError(t, srv.Branch(ctx, "at-head"))
	require.NoError(t, srv.SetBranch(ctx, "rewound", first))
	require.NoError(t, srv.SetRefs(ctx, nil), "no refs is a no-op")
	require.NoError(t, srv.SetRefs(ctx, map[string]string{
		"refs/heads/batched": first,
		"refs/tags/batched":  second,
	}))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	runner := newExecRunner(t, helpers.TmpDirWOSymlinks(t))

	for ref, want := range map[string]string{
		"refs/heads/at-head": second,
		"refs/heads/rewound": first,
		"refs/heads/batched": first,
		"refs/tags/batched":  second,
	} {
		results, err := runner.LsRemote(ctx, url, ref)
		require.NoError(t, err, ref)
		require.Len(t, results, 1, ref)
		assert.Equal(t, want, results[0].Hash, ref)
	}
}

// TestServer_SetConfig pins that a value reaches the served repository, here by hiding a ref from clients.
func TestServer_SetConfig(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	srv := newTestServer(t)

	require.NoError(t, srv.CommitEmpty(ctx, "seed"))
	require.NoError(t, srv.Branch(ctx, "hidden"))
	require.NoError(t, srv.SetConfig(ctx, "transfer.hideRefs", "refs/heads/hidden"))

	url, err := srv.Start(ctx)
	require.NoError(t, err)

	_, err = newExecRunner(t, helpers.TmpDirWOSymlinks(t)).LsRemote(ctx, url, "refs/heads/hidden")
	require.ErrorIs(t, err, git.ErrNoMatchingReference)
}

// TestServer_Routing pins that a mount serves its own repository and any other path serves the main one.
func TestServer_Routing(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	parent := newTestServer(t)
	child := newTestServer(t)

	require.NoError(t, parent.CommitFile(ctx, "parent.txt", []byte("parent"), "parent"))
	require.NoError(t, child.CommitFile(ctx, "child.txt", []byte("child"), "child"))

	parentHead, err := parent.Head(ctx)
	require.NoError(t, err)

	childHead, err := child.Head(ctx)
	require.NoError(t, err)

	assert.Empty(t, parent.BaseURL(), "a server that has not started has no address")

	parent.Mount("/child.git", child)

	_, err = parent.Start(ctx)
	require.NoError(t, err)

	runner := newExecRunner(t, helpers.TmpDirWOSymlinks(t))

	for path, want := range map[string]string{
		"/child.git":     childHead,
		"/repo.git":      parentHead,
		"/any-other.git": parentHead,
	} {
		results, err := runner.LsRemote(ctx, parent.BaseURL()+path, "HEAD")
		require.NoError(t, err, path)
		require.NotEmpty(t, results, path)
		assert.Equal(t, want, results[0].Hash, path)
	}
}

// TestServer_MountPathTaken pins that two mounts on one path fail at Start.
func TestServer_MountPathTaken(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	other := newTestServer(t)

	srv.Mount("/child.git", other)
	srv.Mount("/child.git", other)

	_, err := srv.Start(t.Context())
	require.ErrorIs(t, err, os.ErrExist)
}

// TestServer_CloseWithoutStart pins that closing a server that never listened succeeds.
func TestServer_CloseWithoutStart(t *testing.T) {
	t.Parallel()

	srv, err := git.NewServer()
	require.NoError(t, err)

	require.NoError(t, srv.Close())
}

// TestServer_CanceledContext pins that a git command that cannot start reaches the caller of every operation.
func TestServer_CanceledContext(t *testing.T) {
	t.Parallel()

	const hash = "deadbeefcafefacedeadbeefcafefacedeadbeef"

	testCases := []struct {
		invoke func(ctx context.Context, srv *git.Server) error
		name   string
	}{
		{
			name: "commit file",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFile(ctx, "file.txt", []byte("content"), "msg")
			},
		},
		{
			name: "commit symlink",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitSymlink(ctx, "link", "target", "msg")
			},
		},
		{
			name: "commit registered submodule",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitSubmodule(ctx, "sub", "https://example.com/sub.git", hash, "msg")
			},
		},
		{
			name: "commit unregistered submodule",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitSubmodule(ctx, "sub", "", hash, "msg")
			},
		},
		{
			name: "commit empty",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitEmpty(ctx, "msg")
			},
		},
		{
			name: "commit empty chain",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitEmptyChain(ctx, 1, "msg")
			},
		},
		{
			name: "commit files",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFiles(ctx, map[string][]byte{"file.txt": []byte("content")}, "msg")
			},
		},
		{
			name: "head",
			invoke: func(ctx context.Context, srv *git.Server) error {
				_, err := srv.Head(ctx)
				return err
			},
		},
		{
			name: "tag",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.Tag(ctx, "v1.0.0")
			},
		},
		{
			name: "delete tag",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.DeleteTag(ctx, "v1.0.0")
			},
		},
		{
			name: "branch",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.Branch(ctx, "dev")
			},
		},
		{
			name: "set refs",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.SetRefs(ctx, map[string]string{"refs/heads/dev": hash})
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			require.ErrorIs(t, tc.invoke(ctx, newTestServer(t)), context.Canceled)
		})
	}
}

// TestServer_RejectedWrites pins that a path that cannot be written and a commit with no changes are both errors.
func TestServer_RejectedWrites(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		invoke func(ctx context.Context, srv *git.Server) error
		name   string
	}{
		{
			name: "file under a file",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFile(ctx, "file.txt/nested", []byte("content"), "msg")
			},
		},
		{
			name: "file over a directory",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFile(ctx, "dir", []byte("content"), "msg")
			},
		},
		{
			name: "file with no changes",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFile(ctx, "file.txt", []byte("content"), "msg")
			},
		},
		{
			name: "symlink under a file",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitSymlink(ctx, "file.txt/link", "target", "msg")
			},
		},
		{
			name: "symlink over a file",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitSymlink(ctx, "file.txt", "target", "msg")
			},
		},
		{
			name: "files under a file",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFiles(ctx, map[string][]byte{"file.txt/nested": []byte("content")}, "msg")
			},
		},
		{
			name: "files over a directory",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFiles(ctx, map[string][]byte{"dir": []byte("content")}, "msg")
			},
		},
		{
			name: "files with no changes",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitFiles(ctx, map[string][]byte{"file.txt": []byte("content")}, "msg")
			},
		},
		{
			name: "submodule with a malformed hash",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.CommitSubmodule(ctx, "sub", "", "not-a-hash", "msg")
			},
		},
		{
			name: "ref update with a malformed hash",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.SetRefs(ctx, map[string]string{"refs/heads/dev": "not-a-hash"})
			},
		},
		{
			name: "config without a section",
			invoke: func(ctx context.Context, srv *git.Server) error {
				return srv.SetConfig(ctx, "nosection", "value")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := newTestServer(t)

			require.NoError(t, srv.CommitFiles(t.Context(), map[string][]byte{
				"file.txt":   []byte("content"),
				"dir/nested": []byte("content"),
			}, "seed"))

			require.Error(t, tc.invoke(t.Context(), srv))
		})
	}
}

// newTestServer returns a Server removed when the test ends.
func newTestServer(t *testing.T) *git.Server {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })

	return srv
}

// newExecRunner returns a runner bound to dir that spawns the real git.
func newExecRunner(t *testing.T, dir string) *git.GitRunner {
	t.Helper()

	runner, err := git.NewGitRunner(venv.OSVenv())
	require.NoError(t, err)

	return runner.WithWorkDir(dir)
}

// cloneServer starts srv and returns a fresh clone of it.
func cloneServer(t *testing.T, srv *git.Server) string {
	t.Helper()

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	cloneDir := helpers.TmpDirWOSymlinks(t)

	require.NoError(t, newExecRunner(t, cloneDir).Clone(t.Context(), url, false, 0, ""))

	return cloneDir
}

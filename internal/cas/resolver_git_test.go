package cas_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitResolver_ProbeHEAD(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))

	headHash, err := srv.Head()
	require.NoError(t, err)

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	r := &cas.GitResolver{Venv: cas.Venv{Git: newGitRunner(t)}}

	// Empty Branch → resolver queries HEAD.
	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, headHash, got, "Probe(HEAD) must return the canonical commit SHA verbatim")
}

func TestGitResolver_ProbeBranch(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))
	require.NoError(t, srv.Branch("feature"))

	branchHash, err := srv.Head()
	require.NoError(t, err)

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	r := &cas.GitResolver{Venv: cas.Venv{Git: newGitRunner(t)}, Branch: "feature"}

	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Equal(t, branchHash, got)
}

func TestGitResolver_ProbeTag(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))
	require.NoError(t, srv.Tag("v1.0.0"))

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	r := &cas.GitResolver{Venv: cas.Venv{Git: newGitRunner(t)}, Branch: "v1.0.0"}

	// Annotated-tag ls-remote returns the tag object's hash, not the
	// commit it points to. We just assert the resolver returns a
	// SHA-shaped string; the specific value is whatever git computed.
	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	assert.Len(t, got, 40)
}

func TestGitResolver_CommitFormRefReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))

	commitSHA, err := srv.Head()
	require.NoError(t, err)

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	r := &cas.GitResolver{Venv: cas.Venv{Git: newGitRunner(t)}, Branch: commitSHA}

	// ls-remote does not resolve raw SHAs as refs; the caller passes
	// a commit-form ref directly. Probe must surface this as
	// ErrNoVersionMetadata so the fetcher canonicalizes via rev-parse.
	_, err = r.Probe(t.Context(), url)
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestGitResolver_UnknownRefReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	r := &cas.GitResolver{Venv: cas.Venv{Git: newGitRunner(t)}, Branch: "does-not-exist"}

	_, err = r.Probe(t.Context(), url)
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

func TestGitResolver_TokenIsCacheKeyVerbatim(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))

	headHash, err := srv.Head()
	require.NoError(t, err)

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	r := &cas.GitResolver{Venv: cas.Venv{Git: newGitRunner(t)}}

	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err)
	// GitResolver returns the commit SHA itself; SourceCacheKey would
	// be a no-op on top of this. Any future change that pre-hashes
	// the token would break the git fetcher's use of the returned key
	// as a git object name, so this contract is worth pinning.
	assert.Len(t, got, 40, "git SHA-1 must surface as a 40-char hex string")
	assert.Equal(t, headHash, got)
}

// TestGitResolver_FullSHAHitsLocalCacheOffline pins the offline
// fast path. After EnsureCommit warms the local GitStore, Probe must
// resolve a full-SHA ref without contacting the remote, verified by
// shutting the test server down before calling Probe.
func TestGitResolver_FullSHAHitsLocalCacheOffline(t *testing.T) {
	t.Parallel()

	srv := newEmptyTestServer(t)
	require.NoError(t, srv.CommitFile("README.md", []byte("hi"), "init"))

	headHash, err := srv.Head()
	require.NoError(t, err)

	url, err := srv.Start(t.Context())
	require.NoError(t, err)

	store, v, _ := newTestGitStore(t)
	l := logger.CreateLogger()

	repo, err := store.EnsureCommit(t.Context(), l, v, url, headHash, "")
	require.NoError(t, err)
	require.NoError(t, repo.Unlock())

	require.NoError(t, srv.Close())

	r := &cas.GitResolver{Venv: v, Store: store, Branch: headHash}

	got, err := r.Probe(t.Context(), url)
	require.NoError(t, err, "fast path must skip ls-remote when commit is cached")
	assert.Equal(t, headHash, got)
}

// TestGitResolver_ProbeSCPURLWithBranchUsesSeparateArgs pins the
// regression where Probe glued ?ref=<branch> onto an SCP-form URL
// (`git@host:path`) and then handed the result to net/url.Parse,
// which rejects SCP form. Probe silently lost the branch and called
// `git ls-remote 'git@host:path?ref=feature' HEAD`. Branch must
// travel as a separate ls-remote argument, leaving the SCP URL
// intact.
func TestGitResolver_ProbeSCPURLWithBranchUsesSeparateArgs(t *testing.T) {
	t.Parallel()

	var capturedArgs []string

	runner := newStubGitRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
		capturedArgs = inv.Args
		return vexec.Result{Stdout: []byte("deadbeefcafefacedeadbeefcafefacedeadbeef\trefs/heads/main\n")}
	})

	r := &cas.GitResolver{Venv: cas.Venv{Git: runner}, Branch: "main"}

	_, err := r.Probe(t.Context(), "git@github.com:org/repo.git")
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"ls-remote", "git@github.com:org/repo.git", "main"},
		capturedArgs,
		"SCP URL must reach git as-is with branch passed as a separate ls-remote argument",
	)
}

// TestGitResolver_ProbeHTTPURLWithBranchUsesSeparateArgs covers the
// non-SCP companion of the above: even for URLs net/url.Parse handles
// cleanly, the branch travels as a field on the resolver rather than
// being threaded through the URL.
func TestGitResolver_ProbeHTTPURLWithBranchUsesSeparateArgs(t *testing.T) {
	t.Parallel()

	var capturedArgs []string

	runner := newStubGitRunner(t, func(_ context.Context, inv vexec.Invocation) vexec.Result {
		capturedArgs = inv.Args
		return vexec.Result{Stdout: []byte("deadbeefcafefacedeadbeefcafefacedeadbeef\trefs/heads/main\n")}
	})

	r := &cas.GitResolver{Venv: cas.Venv{Git: runner}, Branch: "main"}

	_, err := r.Probe(t.Context(), "https://example.com/org/repo.git")
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"ls-remote", "https://example.com/org/repo.git", "main"},
		capturedArgs,
		"HTTP URL must reach git without ?ref= glued on; branch is a separate argument",
	)
}

// newGitRunner returns the *git.GitRunner the GitResolver shells out
// through. Tests run against the in-memory git HTTP server defined in
// internal/git so no network is touched.
func newGitRunner(t *testing.T) *git.GitRunner {
	t.Helper()

	r, err := git.NewGitRunner(vexec.NewOSExec())
	require.NoError(t, err)

	return r
}

// newStubGitRunner returns a *git.GitRunner backed by a
// [vexec.NewMemExec] handler so tests can capture the argv passed to
// git without spawning the binary.
func newStubGitRunner(t *testing.T, handler vexec.Handler) *git.GitRunner {
	t.Helper()

	e := vexec.NewMemExec(handler,
		vexec.WithLookPath(func(string) (string, error) { return "git", nil }),
	)

	r, err := git.NewGitRunner(e)
	require.NoError(t, err)

	return r
}

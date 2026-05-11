package getter_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errHgNotFound is the LookPath error the missing-binary tests inject.
var errHgNotFound = errors.New("hg not found")

func TestHgResolver_MissingBinaryReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(
		hgHandler(vexec.Result{}),
		vexec.WithLookPath(func(string) (string, error) { return "", errHgNotFound }),
	)

	r := &getter.HgResolver{Exec: e}

	_, err := r.Probe(t.Context(), "https://example.com/repo")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestHgResolver_BinaryFailureReturnsErrNoVersionMetadata feeds the
// resolver an Exec whose handler returns a non-zero exit code, so the
// resolver swallows the failure as ErrNoVersionMetadata.
func TestHgResolver_BinaryFailureReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(hgHandler(vexec.Result{ExitCode: 1}))

	r := &getter.HgResolver{Exec: e}

	_, err := r.Probe(t.Context(), "https://example.com/repo")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestHgResolver_ParsesNodeFromStubOutput verifies the resolver picks
// up the hex hash from a successful command and wraps it in a
// content-addressed cache key. The stub returns the full 40-char node
// hash the resolver must request (the abbreviated 12-char form has
// ~280M values and is not collision-safe for use as a cache key).
func TestHgResolver_ParsesNodeFromStubOutput(t *testing.T) {
	t.Parallel()

	const fullNode = "abcdef0123456789abcdef0123456789abcdef01"

	e := vexec.NewMemExec(hgHandler(vexec.Result{Stdout: []byte(fullNode + "\n")}))

	r := &getter.HgResolver{Exec: e}

	got, err := r.Probe(t.Context(), "https://example.com/repo?rev=tip")
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("hg-node", fullNode), got)
}

func TestHgResolver_EmptyOutputReturnsErrNoVersionMetadata(t *testing.T) {
	t.Parallel()

	e := vexec.NewMemExec(hgHandler(vexec.Result{Stdout: []byte("\n")}))

	r := &getter.HgResolver{Exec: e}

	_, err := r.Probe(t.Context(), "https://example.com/repo")
	require.ErrorIs(t, err, cas.ErrNoVersionMetadata)
}

// TestHgResolver_PassesRevAsArg pins the argv shape. `--template
// '{node}\n'` is needed for the full 40-char node hash (the cache
// key); `--id` returns the 12-char short form and is not
// collision-safe. `--rev=<v>` and the `--` URL terminator block a
// `-`-prefixed value from being reparsed as an hg flag.
func TestHgResolver_PassesRevAsArg(t *testing.T) {
	t.Parallel()

	var gotArgs []string

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		gotArgs = inv.Args
		return vexec.Result{Stdout: []byte("abcdef0123456789abcdef0123456789abcdef01\n")}
	}

	r := &getter.HgResolver{Exec: vexec.NewMemExec(handler)}

	_, err := r.Probe(t.Context(), "https://example.com/repo?rev=feature-x")
	require.NoError(t, err)

	assert.Equal(t,
		[]string{"identify", "--template", "{node}\n", "--rev=feature-x", "--", "https://example.com/repo"},
		gotArgs,
	)
}

// TestHgResolver_FlagLikeRevStaysBoundToOption pins that a
// `-`-prefixed rev value stays inside the --rev argv element instead
// of appearing as its own flag-shaped element.
func TestHgResolver_FlagLikeRevStaysBoundToOption(t *testing.T) {
	t.Parallel()

	var gotArgs []string

	handler := func(_ context.Context, inv vexec.Invocation) vexec.Result {
		gotArgs = inv.Args
		return vexec.Result{Stdout: []byte("abcdef0123456789abcdef0123456789abcdef01\n")}
	}

	r := &getter.HgResolver{Exec: vexec.NewMemExec(handler)}

	_, err := r.Probe(t.Context(), "https://example.com/repo?rev=--debugger")
	require.NoError(t, err)

	assert.Contains(t, gotArgs, "--rev=--debugger",
		"flag-like rev value must stay bound to --rev in a single argv element")
	assert.NotContains(t, gotArgs, "--debugger",
		"flag-like rev value must not appear as its own argv element")
}

// TestHgResolver_AgainstRealHg verifies the resolver against the
// actual hg binary when it is installed. It uses a freshly-initialized
// repository on disk so the test does not reach the network. The
// assertion pins the resolver's key against a ContentKey derived
// from the full 40-char node hash; this regresses if the resolver
// reverts to `--id`'s 12-char short form.
func TestHgResolver_AgainstRealHg(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg binary not installed on this host")
	}

	repoDir := t.TempDir()

	hg := func(args ...string) {
		cmd := exec.CommandContext(t.Context(), "hg", args...)
		cmd.Dir = repoDir

		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "hg %v failed: %s", args, string(out))
	}

	hg("init", ".")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.tf"), []byte("hello\n"), 0o644))
	hg("--config", "ui.username=test <test@test>", "commit", "-A", "-m", "initial")

	// Independently query the full 40-char node hash so the assertion
	// reflects what the resolver should be folding into the key.
	nodeCmd := exec.CommandContext(t.Context(), "hg", "identify", "--template", "{node}\n", repoDir)
	out, err := nodeCmd.Output()
	require.NoError(t, err)

	fullNode := strings.TrimSpace(string(out))
	require.Len(t, fullNode, 40, "hg must emit a 40-char node hash with --template '{node}'")

	r := getter.NewHgResolver()

	got, err := r.Probe(t.Context(), repoDir)
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("hg-node", fullNode), got)
}

// hgHandler returns a vexec.Handler that always produces the given
// Result, regardless of invocation arguments. Used to pin
// stdout/exit-code on a per-test basis.
func hgHandler(r vexec.Result) vexec.Handler {
	return func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return r
	}
}

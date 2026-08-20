//go:build exec

// Real-hg parity test for HgResolver. It spawns the actual hg binary and
// builds a repository on disk, which costs orders of magnitude more than the
// in-memory exec tests in resolver_hg_test.go, so it is gated behind the exec
// tag alongside the other real-binary parity suites. The default build pins
// the same contracts through the in-memory exec.

package getter_test

import (
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

// TestExecHgResolver_AgainstRealHg verifies the resolver against the
// actual hg binary when it is installed. It uses a freshly-initialized
// repository on disk so the test does not reach the network. The
// assertion pins the resolver's key against a ContentKey derived
// from the full 40-char node hash reported by the stable
// `hg log -T {node}` template API; this regresses if the resolver
// reverts to `--id`'s 12-char short form.
func TestExecHgResolver_AgainstRealHg(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("hg"); err != nil {
		t.Skip("hg binary not installed on this host")
	}

	repoDir := t.TempDir()

	hg := func(args ...string) string {
		cmd := exec.CommandContext(t.Context(), "hg", args...)
		cmd.Dir = repoDir

		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "hg %v failed: %s", args, string(out))

		return string(out)
	}

	hg("init", ".")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "main.tf"), []byte("hello\n"), 0o644))
	hg("--config", "ui.username=test <test@test>", "commit", "-A", "-m", "initial")

	// The template API is a stable hg contract, unlike debug output text.
	fullNode := strings.TrimSpace(hg("log", "-r", "tip", "-T", "{node}"))
	require.Len(t, fullNode, 40, "hg log -T {node} must report a full 40-char node hash")

	r := getter.NewHgResolver(vexec.NewOSExec())

	got, err := r.Probe(t.Context(), repoDir+"?rev=tip")
	require.NoError(t, err)
	assert.Equal(t, cas.ContentKey("hg-node", fullNode), got)
}

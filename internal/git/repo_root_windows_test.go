//go:build windows

package git_test

import (
	"os"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoRepoRootReturnsOSNativePathOnWindows pins that the resolved root
// carries Windows separators. The other HCL path functions return OS-native
// paths, so get_repo_root has to match them.
func TestGoRepoRootReturnsOSNativePathOnWindows(t *testing.T) {
	t.Parallel()

	ctx := cache.ContextWithCache(t.Context())

	// The suite's own directory stands in for the working directory the CLI
	// would have absolutized.
	wd, err := os.Getwd()
	require.NoError(t, err)

	repoRoot, err := git.GoRepoRoot(ctx, venv.OSVenv(), wd)
	require.NoError(t, err)
	require.NotEmpty(t, repoRoot)

	assert.NotContains(t, repoRoot, "/", "expected OS-native path on Windows, got %q", repoRoot)
	assert.Contains(
		t,
		repoRoot,
		"\\",
		"expected backslash separators on Windows, got %q",
		strings.ReplaceAll(repoRoot, "\\", "\\\\"),
	)
}

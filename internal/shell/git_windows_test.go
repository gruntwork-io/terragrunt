//go:build windows

package shell_test

import (
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitTopLevelDirReturnsOSNativePathOnWindows guards against a regression
// where `git rev-parse --show-toplevel` output (always forward-slashed, even
// on Windows) leaked through unchanged. The other HCL path functions return
// OS-native paths, so get_repo_root must too.
func TestGitTopLevelDirReturnsOSNativePathOnWindows(t *testing.T) {
	t.Parallel()

	ctx := cache.ContextWithCache(t.Context())

	terragruntOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	repoRoot, err := shell.GitTopLevelDir(ctx, l, vexec.NewOSExec(), terragruntOptions.Env, ".")
	require.NoError(t, err)
	require.NotEmpty(t, repoRoot)

	assert.NotContains(t, repoRoot, "/", "expected OS-native path on Windows, got %q", repoRoot)
	assert.Contains(t, repoRoot, "\\", "expected backslash separators on Windows, got %q", strings.ReplaceAll(repoRoot, "\\", "\\\\"))
}

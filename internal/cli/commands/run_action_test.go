package commands_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunActionInstallsRunScopedCache pins the contract that RunAction
// installs the run-scoped cache on the context handed to the action.
// Without this, a future refactor could quietly drop the cache wiring and
// regress the optimization that 6019 introduced.
func TestRunActionInstallsRunScopedCache(t *testing.T) {
	t.Parallel()

	var (
		hasRunCmd    bool
		hasRepoRoots bool
	)

	action := func(ctx context.Context, _ *clihelper.Context) error {
		_, hasRunCmd = ctx.Value(cache.RunCmdCacheContextKey).(*cache.Cache[string])
		_, hasRepoRoots = ctx.Value(cache.RepoRootCacheContextKey).(*cache.RepoRootCache)

		return nil
	}

	opts := options.NewTerragruntOptions()
	opts.NoAutoProviderCacheDir = true

	l := logger.CreateLogger()

	require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, venv.OSVenv(), action))
	assert.True(t, hasRunCmd, "RunCmdCacheContextKey missing from action context")
	assert.True(t, hasRepoRoots, "RepoRootCacheContextKey missing from action context")
}

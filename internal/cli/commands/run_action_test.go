package commands_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/panicreport"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errCacheDir = errors.New("no user cache dir")

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

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	l := logger.CreateLogger()

	require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, venvtest.New(), action))
	assert.True(t, hasRunCmd, "RunCmdCacheContextKey missing from action context")
	assert.True(t, hasRepoRoots, "RepoRootCacheContextKey missing from action context")
}

func TestRunActionReturnsReportableErrorOnActionPanic(t *testing.T) {
	t.Parallel()

	action := func(_ context.Context, _ *clihelper.Context) error {
		panic("action panic")
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true

	err := commands.RunAction(t.Context(), nil, logger.CreateLogger(), opts, venvtest.New(), action)
	require.Error(t, err)
	assert.True(t, panicreport.IsPanic(err), "returned error should be classified as a panic")

	msg, stack := panicreport.PanicDetails(err)
	assert.Equal(t, "action panic", msg)
	assert.Contains(t, string(stack), "run_action_test.go")
}

// TestRunActionProviderCacheStartupFailures pins that a provider cache server that cannot start fails the run before its action.
func TestRunActionProviderCacheStartupFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		venv     func(v *venv.Venv) *venv.Venv
		wantErr  error
		name     string
		cacheDir string
	}{
		{
			name:     "listener refused",
			cacheDir: "/virtual/provider-cache",
			wantErr:  venvtest.ErrNoListen,
		},
		{
			name: "cache dir unresolvable",
			venv: func(v *venv.Venv) *venv.Venv {
				return v.WithUserCacheDir(func() (string, error) { return "", errCacheDir })
			},
			wantErr: errCacheDir,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithGOOS("linux").WithHandler(fakeTerraformVersionHandler)
			if tc.venv != nil {
				v = tc.venv(v)
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.NoAutoProviderCacheDir = true
			opts.TFPath = "terraform"
			opts.ProviderCacheOptions.Enabled = true
			opts.ProviderCacheOptions.Dir = tc.cacheDir
			opts.ProviderCacheOptions.Hostname = "127.0.0.1"
			opts.ProviderCacheOptions.Token = "11111111-2222-3333-4444-555555555555"

			actionRan := false

			err := commands.RunAction(config.WithCaches(t.Context()), nil, logger.CreateLogger(), opts, v,
				func(context.Context, *clihelper.Context) error {
					actionRan = true

					return nil
				})
			require.ErrorIs(t, err, tc.wantErr)
			assert.False(t, actionRan, "the action must not run without the provider cache server")
		})
	}
}

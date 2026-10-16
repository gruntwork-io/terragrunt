package prepare_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeOpenTofuVersionHandler answers a version probe the way an OpenTofu binary would.
func fakeOpenTofuVersionHandler(ver string) vexec.Handler {
	return func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("OpenTofu v" + ver + "\non linux_amd64\n")}
	}
}

// TestSetupAutoProviderCacheDir pins the conditions under which the setup
// exports TF_PLUGIN_CACHE_DIR, now that only run paths invoke it (#7094).
func TestSetupAutoProviderCacheDir(t *testing.T) {
	t.Parallel()

	const cacheDir = "/virtual/provider-cache"

	newOpts := func() *options.TerragruntOptions {
		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TFPath = "tofu"
		opts.ProviderCacheOptions.Dir = cacheDir

		return opts
	}

	t.Run("opt-out skips without probing", func(t *testing.T) {
		t.Parallel()

		opts := newOpts()
		opts.ProviderCacheOptions.NoAutoCacheDir = true

		// venvtest's fail-closed exec errors on any spawn, so success proves no probe ran.
		v := venvtest.New().WithGOOS("linux")

		require.NoError(t, prepare.SetupAutoProviderCacheDir(t.Context(), logger.CreateLogger(), v, opts))
		assert.Empty(t, v.Env[tf.EnvNameTFPluginCacheDir])
	})

	t.Run("deprecated TerragruntOptions opt-out still skips without probing", func(t *testing.T) {
		t.Parallel()

		opts := newOpts()
		opts.NoAutoProviderCacheDir = true //nolint:staticcheck // Pins the deprecated Go API field keeps working.

		v := venvtest.New().WithGOOS("linux")

		require.NoError(t, prepare.SetupAutoProviderCacheDir(t.Context(), logger.CreateLogger(), v, opts))
		assert.Empty(t, v.Env[tf.EnvNameTFPluginCacheDir])
	})

	t.Run("preset TF_PLUGIN_CACHE_DIR skips without probing", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithGOOS("linux")
		v.Env[tf.EnvNameTFPluginCacheDir] = "/user/cache"

		require.NoError(t, prepare.SetupAutoProviderCacheDir(t.Context(), logger.CreateLogger(), v, newOpts()))
		assert.Equal(t, "/user/cache", v.Env[tf.EnvNameTFPluginCacheDir])
	})

	t.Run("terraform implementation is rejected", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithGOOS("linux").WithHandler(fakeTerraformVersionHandler)

		require.Error(t, prepare.SetupAutoProviderCacheDir(t.Context(), logger.CreateLogger(), v, newOpts()))
		assert.Empty(t, v.Env[tf.EnvNameTFPluginCacheDir])
	})

	t.Run("OpenTofu below 1.10 is rejected", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithGOOS("linux").WithHandler(fakeOpenTofuVersionHandler("1.9.0"))

		require.Error(t, prepare.SetupAutoProviderCacheDir(t.Context(), logger.CreateLogger(), v, newOpts()))
		assert.Empty(t, v.Env[tf.EnvNameTFPluginCacheDir])
	})

	t.Run("OpenTofu 1.10 exports the cache dir", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithGOOS("linux").WithHandler(fakeOpenTofuVersionHandler("1.10.0"))

		// t.TempDir is absolute on every host OS, so filepath.IsAbs and filepath.Clean leave it unchanged.
		dir := t.TempDir()

		opts := newOpts()
		opts.ProviderCacheOptions.Dir = dir

		require.NoError(t, prepare.SetupAutoProviderCacheDir(t.Context(), logger.CreateLogger(), v, opts))
		assert.Equal(t, dir, v.Env[tf.EnvNameTFPluginCacheDir])

		assert.True(t, vfs.Exists(v.FS, dir), "the cache dir must be created on the virtual filesystem")
	})
}

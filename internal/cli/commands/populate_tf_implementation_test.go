package commands_test

import (
	"context"
	"net"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTerraformVersionHandler answers a version probe the way a Terraform binary would;
// tests never assert on the version itself, only on user-visible outcomes.
func fakeTerraformVersionHandler(context.Context, vexec.Invocation) vexec.Result {
	return vexec.Result{Stdout: []byte("Terraform v1.15.3\non linux_amd64\n")}
}

// TestPopulateTFImplementation pins the detection boundary the provider cache server
// relies on: the probed binary's implementation lands on opts before InitServer reads it.
func TestPopulateTFImplementation(t *testing.T) {
	t.Parallel()

	t.Run("probes the binary and stores the result", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithHandler(fakeTerraformVersionHandler)

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TFPath = "terraform"

		require.NoError(t, commands.PopulateTFImplementation(t.Context(), logger.CreateLogger(), opts, v))
		assert.Equal(t, tfimpl.Terraform, opts.TofuImplementation)
		assert.NotNil(t, opts.TerraformVersion)
	})

	t.Run("skips the probe when already populated", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TofuImplementation = tfimpl.OpenTofu
		opts.TerraformVersion = version.Must(version.NewVersion("1.0.0"))

		// venvtest's fail-closed exec errors on any spawn, so success proves no probe ran.
		require.NoError(t, commands.PopulateTFImplementation(t.Context(), logger.CreateLogger(), opts, venvtest.New()))
		assert.Equal(t, tfimpl.OpenTofu, opts.TofuImplementation)
	})

	t.Run("returns the probe error and leaves opts untouched", func(t *testing.T) {
		t.Parallel()

		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.TFPath = "terraform"

		require.Error(t, commands.PopulateTFImplementation(t.Context(), logger.CreateLogger(), opts, venvtest.New()))
		assert.Equal(t, tfimpl.Unknown, opts.TofuImplementation)
		assert.Nil(t, opts.TerraformVersion)
	})
}

// TestRunActionDetectsImplementationForProviderCache pins the RunAction wiring: with the
// provider cache enabled, the implementation is resolved before the cache server starts,
// and a failed probe degrades to OpenTofu's file locations with a warning.
func TestRunActionDetectsImplementationForProviderCache(t *testing.T) {
	t.Parallel()

	newOpts := func() *options.TerragruntOptions {
		opts := options.NewTerragruntOptions(vexec.NewOSExec())
		opts.NoAutoProviderCacheDir = true
		opts.TFPath = "terraform"
		opts.ProviderCacheOptions.Enabled = true
		opts.ProviderCacheOptions.Dir = "/virtual/provider-cache"
		opts.ProviderCacheOptions.Hostname = "127.0.0.1"
		opts.ProviderCacheOptions.Token = "11111111-2222-3333-4444-555555555555"

		return opts
	}

	listen := func(ctx context.Context, network, addr string) (net.Listener, error) {
		var lc net.ListenConfig

		return lc.Listen(ctx, network, addr)
	}

	action := func(context.Context, *clihelper.Context) error { return nil }

	t.Run("stores the detected implementation where InitServer reads it", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().
			WithGOOS("linux").
			WithHandler(fakeTerraformVersionHandler)
		v.Listen = listen

		opts := newOpts()

		l, output := newTestLogger()

		require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, action))
		assert.Equal(t, tfimpl.Terraform, opts.TofuImplementation)
		assert.NotContains(t, output.String(), "falls back to OpenTofu's CLI config file locations")
	})

	t.Run("failed detection warns and falls back", func(t *testing.T) {
		t.Parallel()

		v := venvtest.New().WithGOOS("linux")
		v.Listen = listen

		opts := newOpts()

		l, output := newTestLogger()

		require.NoError(t, commands.RunAction(t.Context(), nil, l, opts, v, action))
		assert.Equal(t, tfimpl.Unknown, opts.TofuImplementation)
		assert.Contains(t, output.String(), "falls back to OpenTofu's CLI config file locations")
	})
}

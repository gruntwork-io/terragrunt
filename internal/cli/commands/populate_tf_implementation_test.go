package commands_test

import (
	"context"
	"net"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
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

// TestRunActionProviderCacheUsesDetectedImplementation pins the InitServer boundary
// end-to-end: the cache server must be initialized with the implementation RunAction
// detected, proven by the generated CLI config a unit's run receives. Hard-coding the
// wrong implementation at the InitServer call site makes the hook bypass the cache and
// fails this test.
func TestRunActionProviderCacheUsesDetectedImplementation(t *testing.T) {
	t.Parallel()

	const (
		home    = "/virtual/home"
		workDir = "/virtual/work"
		mirror  = "https://mirror.example.test/providers/"
	)

	v := venvtest.New().
		WithGOOS("linux").
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithHandler(func(ctx context.Context, inv vexec.Invocation) vexec.Result {
			if slices.Contains(inv.Args, "-version") {
				return fakeTerraformVersionHandler(ctx, inv)
			}

			return vexec.Result{}
		})
	v.Listen = func(ctx context.Context, network, addr string) (net.Listener, error) {
		var lc net.ListenConfig

		return lc.Listen(ctx, network, addr)
	}

	require.NoError(t, v.FS.MkdirAll(home, 0o755))
	require.NoError(t, v.FS.MkdirAll(workDir, 0o755))
	// The .tofurc mirror must never reach a Terraform run's generated CLI config.
	require.NoError(t, vfs.WriteFile(v.FS, home+"/.tofurc", []byte(`
provider_installation {
  network_mirror {
    url = "`+mirror+`"
  }
}
`), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, home+"/.terraformrc", []byte("\n"), 0o600))

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.NoAutoProviderCacheDir = true
	opts.TFPath = "terraform"
	opts.ProviderCacheOptions.Enabled = true
	opts.ProviderCacheOptions.Dir = "/virtual/provider-cache"
	opts.ProviderCacheOptions.Hostname = "127.0.0.1"
	opts.ProviderCacheOptions.Token = "11111111-2222-3333-4444-555555555555"

	action := func(ctx context.Context, _ *clihelper.Context) error {
		hook := tf.TerraformCommandHookFromContext(ctx)
		require.NotNil(t, hook, "RunAction must install the provider cache hook")

		tfOpts := &tf.TFOptions{
			TerraformCliArgs:   iacargs.New(),
			ShellOptions:       shell.NewShellOptions(map[string]string{}).WithTFPath("terraform").WithWorkingDir(workDir),
			TofuImplementation: tfimpl.Terraform,
		}

		_, err := hook(ctx, logger.CreateLogger(), v, tfOpts, clihelper.Args{"init"})

		return err
	}

	require.NoError(t, commands.RunAction(t.Context(), nil, logger.CreateLogger(), opts, v, action))

	generated, err := vfs.ReadFile(v.FS, workDir+"/.terraformrc")
	require.NoError(t, err, "a matching implementation must go through the cache and generate its CLI config")
	assert.NotContains(t, string(generated), mirror)
	assert.Contains(t, string(generated), "registry.terraform.io")
}

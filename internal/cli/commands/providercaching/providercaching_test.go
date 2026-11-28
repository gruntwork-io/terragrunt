package providercaching_test

import (
	"bytes"
	"context"
	"net"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/providercaching"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTerraformVersionHandler answers a version probe the way a Terraform binary would;
// tests never assert on the version itself, only on user-visible outcomes.
func fakeTerraformVersionHandler(context.Context, vexec.Invocation) vexec.Result {
	return vexec.Result{Stdout: []byte("Terraform v1.15.3\non linux_amd64\n")}
}

// TestRunDetectsImplementation pins the Run wiring: with the
// provider cache enabled, the implementation is resolved before the cache server starts,
// and a failed probe degrades to OpenTofu's file locations without failing the run.
func TestRunDetectsImplementation(t *testing.T) {
	t.Parallel()

	newOpts := func() *options.TerragruntOptions {
		opts := options.NewTerragruntOptions()
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

	t.Run("detection succeeds without the fallback message", func(t *testing.T) {
		t.Parallel()

		v := venv.MemVenv().
			WithGOOS("linux").
			WithHandler(fakeTerraformVersionHandler)
		v.Listen = listen

		opts := newOpts()

		l, output := newDebugTestLogger()

		require.NoError(t, runWithProviderCaching(config.WithCaches(t.Context()), l, opts, v, action))
		assert.NotContains(t, output.String(), "falls back to OpenTofu's CLI config file locations")
	})

	t.Run("failed detection falls back without failing the run", func(t *testing.T) {
		t.Parallel()

		v := venv.MemVenv().WithGOOS("linux")
		v.Listen = listen

		opts := newOpts()

		l, output := newDebugTestLogger()

		require.NoError(t, runWithProviderCaching(config.WithCaches(t.Context()), l, opts, v, action))
		assert.Contains(t, output.String(), "falls back to OpenTofu's CLI config file locations")
	})
}

// TestRunUsesDetectedImplementation pins the InitServer boundary
// end-to-end: the cache server must be initialized with the implementation Run
// detected, proven by the generated CLI config a unit's run receives. Hard-coding the
// wrong implementation at the InitServer call site makes the hook bypass the cache and
// fails this test.
func TestRunUsesDetectedImplementation(t *testing.T) {
	t.Parallel()

	const (
		home    = "/virtual/home"
		workDir = "/virtual/work"
		mirror  = "https://mirror.example.test/providers/"
	)

	v := venv.MemVenv().
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

	opts := options.NewTerragruntOptions()
	opts.NoAutoProviderCacheDir = true
	opts.TFPath = "terraform"
	opts.ProviderCacheOptions.Enabled = true
	opts.ProviderCacheOptions.Dir = "/virtual/provider-cache"
	opts.ProviderCacheOptions.Hostname = "127.0.0.1"
	opts.ProviderCacheOptions.Token = "11111111-2222-3333-4444-555555555555"

	action := func(ctx context.Context, _ *clihelper.Context) error {
		hook := tf.TerraformCommandHookFromContext(ctx)
		require.NotNil(t, hook, "Run must install the provider cache hook")

		tfOpts := &tf.TFOptions{
			TerraformCliArgs:   iacargs.New(),
			ShellOptions:       shell.NewShellOptions(map[string]string{}).WithTFPath("terraform").WithWorkingDir(workDir),
			TofuImplementation: tfimpl.Terraform,
		}

		_, err := hook(ctx, logger.CreateLogger(), v, tfOpts, clihelper.Args{"init"})

		return err
	}

	require.NoError(t, runWithProviderCaching(config.WithCaches(t.Context()), logger.CreateLogger(), opts, v, action))

	generated, err := vfs.ReadFile(v.FS, workDir+"/.terraformrc")
	require.NoError(t, err, "a matching implementation must go through the cache and generate its CLI config")
	assert.NotContains(t, string(generated), mirror)
	assert.Contains(t, string(generated), "registry.terraform.io")
}

// newDebugTestLogger returns a debug-level logger whose output the test can assert on;
// a failed implementation probe is a debug detail, not a warning the user must act on.
func newDebugTestLogger() (log.Logger, *bytes.Buffer) {
	formatter := format.NewFormatter(placeholders.Placeholders{placeholders.Message()})
	output := new(bytes.Buffer)

	return log.New(log.WithOutput(output), log.WithLevel(log.DebugLevel), log.WithFormatter(formatter)), output
}

// runWithProviderCaching runs action under [providercaching.Run], on a context that holds the
// run-scoped caches the version probe reads.
func runWithProviderCaching(
	ctx context.Context,
	l log.Logger,
	opts *options.TerragruntOptions,
	v *venv.Venv,
	action clihelper.ActionFunc,
) error {
	return providercaching.Run(cache.ContextWithCache(ctx), nil, l, opts, v, action)
}

// TestRunStartupFailures pins that a provider cache server that cannot start fails the run before its action.
func TestRunStartupFailures(t *testing.T) {
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
			wantErr:  venv.ErrNoListen,
		},
		{
			name: "cache dir unresolvable",
			venv: func(v *venv.Venv) *venv.Venv {
				return v.WithUserCacheDir(func() (string, error) { return "", errInjected })
			},
			wantErr: errInjected,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv().WithGOOS("linux").WithHandler(fakeTerraformVersionHandler)
			if tc.venv != nil {
				v = tc.venv(v)
			}

			opts := options.NewTerragruntOptions()
			opts.NoAutoProviderCacheDir = true
			opts.TFPath = "terraform"
			opts.ProviderCacheOptions.Enabled = true
			opts.ProviderCacheOptions.Dir = tc.cacheDir
			opts.ProviderCacheOptions.Hostname = "127.0.0.1"
			opts.ProviderCacheOptions.Token = "11111111-2222-3333-4444-555555555555"

			actionRan := false

			err := runWithProviderCaching(config.WithCaches(t.Context()), logger.CreateLogger(), opts, v,
				func(context.Context, *clihelper.Context) error {
					actionRan = true

					return nil
				})
			require.ErrorIs(t, err, tc.wantErr)
			assert.False(t, actionRan, "the action must not run without the provider cache server")
		})
	}
}

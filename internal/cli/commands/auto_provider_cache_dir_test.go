package commands_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands"
	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tofu110Version = "OpenTofu v1.10.0\non linux_amd64\n"
	tofu19Version  = "OpenTofu v1.9.0\non linux_amd64\n"
	tf115Version   = "Terraform v1.15.3\non linux_amd64\n"
)

// TestRunActionAutoProviderCacheDir pins when RunAction points TF_PLUGIN_CACHE_DIR at a provider cache dir it creates.
func TestRunActionAutoProviderCacheDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		venv         func(v *venv.Venv) *venv.Venv
		name         string
		versionOut   string
		cacheDir     string
		wantCacheDir string
		wantDefault  bool
	}{
		{
			name:         "opentofu 1.10 uses the configured absolute dir",
			versionOut:   tofu110Version,
			cacheDir:     venvtest.Root("/providers") + filepath.FromSlash("/./cache"),
			wantCacheDir: venvtest.Root("/providers/cache"),
		},
		{
			name:         "relative dir joins the root working dir",
			versionOut:   tofu110Version,
			cacheDir:     "providers",
			wantCacheDir: filepath.Join(testWorkDir, "providers"),
		},
		{
			name:        "unset dir defaults under the user cache dir",
			versionOut:  tofu110Version,
			wantDefault: true,
		},
		{
			name:       "opentofu before 1.10 is skipped",
			versionOut: tofu19Version,
			cacheDir:   venvtest.Root("/providers"),
		},
		{
			name:       "terraform is skipped",
			versionOut: tf115Version,
			cacheDir:   venvtest.Root("/providers"),
		},
		{
			name:     "failed version probe is skipped",
			cacheDir: venvtest.Root("/providers"),
		},
		{
			name:       "user cache dir error is skipped",
			versionOut: tofu110Version,
			venv: func(v *venv.Venv) *venv.Venv {
				return v.WithUserCacheDir(func() (string, error) { return "", errInjected })
			},
		},
		{
			name:       "cache dir creation error is skipped",
			versionOut: tofu110Version,
			cacheDir:   venvtest.Root("/providers"),
			venv: func(v *venv.Venv) *venv.Venv {
				return v.WithFS(newFaultFS(v.FS, atPath("mkdirall", venvtest.Root("/providers"))))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venv.MemVenv()
			if tc.versionOut != "" {
				v = v.WithHandler(versionHandler(tc.versionOut))
			}

			if tc.venv != nil {
				v = tc.venv(v)
			}

			wantCacheDir := tc.wantCacheDir

			if tc.wantDefault {
				userCacheDir, err := v.Platform.UserCacheDir()
				require.NoError(t, err)

				wantCacheDir = filepath.Join(userCacheDir, "terragrunt", "providers")
			}

			opts := options.NewTerragruntOptions(vexec.NewOSExec())
			opts.TFPath = "tofu"
			opts.RootWorkingDir = testWorkDir
			opts.ProviderCacheOptions.Dir = tc.cacheDir

			require.NoError(
				t,
				commands.RunAction(config.WithCaches(t.Context()), nil, logger.CreateLogger(), opts, v, noopAction),
			)

			if wantCacheDir == "" {
				assert.NotContains(t, v.Env, tf.EnvNameTFPluginCacheDir, "TF_PLUGIN_CACHE_DIR should stay unset")

				return
			}

			assert.Equal(
				t,
				wantCacheDir,
				v.Env[tf.EnvNameTFPluginCacheDir],
				"TF_PLUGIN_CACHE_DIR should name the provider cache dir",
			)
			assert.True(t, vfs.IsDir(v.FS, wantCacheDir), "provider cache dir %s should be created", wantCacheDir)
		})
	}
}

// TestRunActionAutoProviderCacheDirKeepsUserValue pins that a TF_PLUGIN_CACHE_DIR the user set is kept without probing the binary.
func TestRunActionAutoProviderCacheDirKeepsUserValue(t *testing.T) {
	t.Parallel()

	probes := 0

	v := venv.MemVenv().WithHandler(func(ctx context.Context, inv vexec.Invocation) vexec.Result {
		probes++

		return versionHandler(tofu110Version)(ctx, inv)
	})
	v.Env[tf.EnvNameTFPluginCacheDir] = "/user/plugins"

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.TFPath = "tofu"
	opts.ProviderCacheOptions.Dir = venvtest.Root("/providers")

	require.NoError(t, commands.RunAction(config.WithCaches(t.Context()), nil, logger.CreateLogger(), opts, v, noopAction))
	assert.Equal(t, "/user/plugins", v.Env[tf.EnvNameTFPluginCacheDir], "user value should win")
	assert.Zero(t, probes, "the binary should not be probed")
	assert.False(t, vfs.IsDir(v.FS, opts.ProviderCacheOptions.Dir), "no provider cache dir should be created")
}

// versionHandler answers every invocation with the given version output.
func versionHandler(out string) vexec.Handler {
	return func(context.Context, vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(out)}
	}
}

// noopAction is a command action that does nothing.
func noopAction(context.Context, *clihelper.Context) error {
	return nil
}

package prepare_test

import (
	"context"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/prepare"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestPrepareConfigReadsThroughTheStore(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "unit", config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
locals {
  region = "us-east-1"
}

inputs = {
  region = local.region
}
`), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	store := new(config.Store)

	opts, err := options.NewTerragruntOptionsWithConfigPath(path)
	require.NoError(t, err)

	prepared, err := prepare.PrepareConfig(config.WithCaches(t.Context()), logger.CreateLogger(), v, store, opts)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"region": "us-east-1"}, prepared.Cfg.Inputs)
	assert.NotZero(t, store.Stats().Misses)
}

func TestPrepareSourceInPlaceKeepsWorkingDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cfg            *config.TerragruntConfig
		name           string
		configDirFiles []string
	}{
		{
			name: "config directory without code",
			cfg:  new(config.TerragruntConfig),
		},
		{
			name:           "config directory with code",
			cfg:            new(config.TerragruntConfig),
			configDirFiles: []string{"main.tf"},
		},
		{
			name: "generate block",
			cfg: &config.TerragruntConfig{
				GenerateConfigs: map[string]codegen.GenerateConfig{"main": {Path: "main.tf"}},
			},
		},
		{
			name: "remote_state generate",
			cfg: &config.TerragruntConfig{
				RemoteState: remotestate.New(&remotestate.Config{
					BackendName: "local",
					Generate:    &remotestate.ConfigGenerate{Path: "backend.tf"},
				}),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			unitDir := filepath.Join(root, "unit")
			configDir := filepath.Join(root, "configs")
			configPath := filepath.Join(configDir, config.DefaultTerragruntConfigPath)

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, configPath, []byte(""), 0o644))
			require.NoError(t, vfs.WriteFile(fsys, filepath.Join(unitDir, "main.tf"), []byte(""), 0o644))

			for _, name := range tc.configDirFiles {
				require.NoError(t, vfs.WriteFile(fsys, filepath.Join(configDir, name), []byte(""), 0o644))
			}

			opts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
			require.NoError(t, err)

			opts.WorkingDir = unitDir

			updatedOpts := prepareSourceInPlace(t, fsys, opts, tc.cfg)

			assert.Equal(t, unitDir, updatedOpts.WorkingDir)
			assert.NotSame(t, opts, updatedOpts)
		})
	}
}

func TestPrepareSourceInPlaceKeepsWorkingDirWithRelativeConfigPath(t *testing.T) {
	t.Parallel()

	unitDir := filepath.Join(t.TempDir(), "unit")

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(unitDir, "main.tf"), []byte(""), 0o644))

	opts, err := options.NewTerragruntOptionsWithConfigPath(filepath.Join(unitDir, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	opts.TerragruntConfigPath = filepath.Join("config", config.DefaultTerragruntConfigPath)

	updatedOpts := prepareSourceInPlace(t, fsys, opts, new(config.TerragruntConfig))

	assert.Equal(t, unitDir, updatedOpts.WorkingDir)
}

func TestPrepareSourceInPlaceReturnsAbsoluteConfigPath(t *testing.T) {
	t.Parallel()

	unitDir := t.TempDir()
	configPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(unitDir, "main.tf"), []byte(""), 0o644))

	opts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
	require.NoError(t, err)

	opts.TerragruntConfigPath = config.DefaultTerragruntConfigPath

	updatedOpts := prepareSourceInPlace(t, fsys, opts, new(config.TerragruntConfig))

	assert.Equal(t, configPath, updatedOpts.TerragruntConfigPath)
}

func TestPrepareInitInCacheReplacesLeftoverNullVarsFile(t *testing.T) {
	t.Parallel()

	unitDir := t.TempDir()
	fsys := vfs.NewOSFS()
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(unitDir, config.DefaultTerragruntConfigPath), nil, 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(unitDir, "main.tf"), nil, 0o644))

	opts, err := options.NewTerragruntOptionsWithConfigPath(filepath.Join(unitDir, config.DefaultTerragruntConfigPath))
	require.NoError(t, err)

	var initRan atomic.Bool

	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(initRecorder(&initRan)))
	ctx := run.WithRunVersionCache(t.Context())
	l := logger.CreateLogger()
	r := report.NewReport()
	cfg := &config.TerragruntConfig{Inputs: map[string]any{"var1": nil}}

	updatedOpts, impl, err := prepare.PrepareSource(ctx, l, v, opts, cfg, r)
	require.NoError(t, err)
	require.NotEqual(t, unitDir, updatedOpts.WorkingDir)

	leftover := filepath.Join(updatedOpts.WorkingDir, run.NullTFVarsFile)
	require.NoError(t, vfs.WriteFile(fsys, leftover, []byte(`{"var1":`), 0o600))

	require.NoError(t, prepare.PrepareInit(ctx, l, v, opts, updatedOpts, impl, cfg.ToRunConfig(l, fsys), r))

	assert.True(t, initRan.Load(), "init must run")
	assert.NoFileExists(t, leftover)
}

func TestPrepareInitInPlaceKeepsNullVarsFileWhenConfigIsElsewhere(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	unitDir := filepath.Join(root, "unit")
	configPath := filepath.Join(root, "config", config.DefaultTerragruntConfigPath)
	nullVarsPath := filepath.Join(unitDir, run.NullTFVarsFile)

	fsys := vfs.NewOSFS()
	require.NoError(t, vfs.WriteFile(fsys, configPath, nil, 0o644))
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(unitDir, "main.tf"), nil, 0o644))
	require.NoError(t, vfs.WriteFile(fsys, nullVarsPath, []byte(`{"keep": "original"}`), 0o644))

	opts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
	require.NoError(t, err)

	opts.WorkingDir = unitDir
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))

	var initRan atomic.Bool

	v := venv.MemVenv().WithFS(fsys).WithExec(vexec.NewMemExec(initRecorder(&initRan)))
	ctx := run.WithRunVersionCache(t.Context())
	l := logger.CreateLogger()
	r := report.NewReport()
	cfg := &config.TerragruntConfig{
		Terraform: &config.TerraformConfig{NoDotTerragruntCache: new(true)},
		Inputs:    map[string]any{"var1": nil},
	}

	updatedOpts, impl, err := prepare.PrepareSource(ctx, l, v, opts, cfg, r)
	require.NoError(t, err)

	err = prepare.PrepareInit(ctx, l, v, opts, updatedOpts, impl, cfg.ToRunConfig(l, fsys), r)

	var exists run.VarsFileExists

	require.ErrorAs(t, err, &exists)
	assert.Equal(t, nullVarsPath, exists.Path)
	assert.False(t, initRan.Load(), "init must not run")

	contents, err := vfs.ReadFile(fsys, nullVarsPath)
	require.NoError(t, err)
	assert.JSONEq(t, `{"keep": "original"}`, string(contents))
}

func prepareSourceInPlace(
	t *testing.T,
	fsys vfs.FS,
	opts *options.TerragruntOptions,
	cfg *config.TerragruntConfig,
) *options.TerragruntOptions {
	t.Helper()

	require.NoError(t, opts.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))

	cfg.Terraform = &config.TerraformConfig{NoDotTerragruntCache: new(true)}

	updatedOpts, _, err := prepare.PrepareSource(
		run.WithRunVersionCache(t.Context()),
		logger.CreateLogger(),
		venv.MemVenv().WithFS(fsys),
		opts,
		cfg,
		report.NewReport(),
	)
	require.NoError(t, err)

	return updatedOpts
}

// initRecorder returns an exec handler that sets initRan when it runs an init command.
func initRecorder(initRan *atomic.Bool) vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if slices.Contains(inv.Args, "init") {
			initRan.Store(true)
		}

		return vexec.Result{}
	}
}

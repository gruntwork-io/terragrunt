package run_test

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file drive run.Run against a mem-backed exec, so the in-place behavior of
// terraform.no_dot_terragrunt_cache is pinned without a real tofu binary.

func TestRunNoDotTerragruntCacheRunsInUnitDir(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	opts := newNoDotTerragruntCacheOpts(t, s, "plan")
	cfg := &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true}}

	require.NoError(t, run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()))

	calls := rec.snapshot()
	require.Len(t, calls, 1)
	assert.Equal(t, "tofu", calls[0].Name)
	assert.Equal(t, s.dir, calls[0].Dir, "tofu must run in the unit directory")
	assert.NoDirExists(t, filepath.Join(s.dir, ".terragrunt-cache"))
}

func TestRunNoDotTerragruntCacheWithoutExperimentReturnsError(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	opts := newRunE2EOpts(t, s, "plan")
	cfg := &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true}}

	err := run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter())

	var experimentErr runcfg.NoDotTerragruntCacheRequiresExperimentError

	require.ErrorAs(t, err, &experimentErr)
	assert.Equal(t, s.configPath, experimentErr.ConfigPath)
	assert.Empty(t, rec.snapshot(), "tofu must not run")
}

func TestRunNoDotTerragruntCacheWithSourceReturnsError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		cliSource    string
		configSource string
	}{
		{name: "source in the terraform block", configSource: "./mod"},
		{name: "source from the --source flag", cliSource: "./mod"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			rec := &recorder{}
			v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

			opts := newNoDotTerragruntCacheOpts(t, s, "plan")
			opts.Source = tc.cliSource
			cfg := &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true, Source: tc.configSource},
			}

			err := run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter())

			var sourceErr runcfg.NoDotTerragruntCacheWithSourceError

			require.ErrorAs(t, err, &sourceErr)
			assert.Equal(t, "./mod", sourceErr.Source)
			assert.Empty(t, rec.snapshot(), "tofu must not run")
		})
	}
}

func TestRunNoDotTerragruntCacheInitKeepsSymlinkedLockFile(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)

	// The unit shares its lock file with a sibling directory. In place, the post-init lock-file copy has the unit
	// directory as both source and destination and must leave the symlink alone.
	sharedDir := filepath.Join(filepath.Dir(s.dir), "shared-lock")
	require.NoError(t, os.MkdirAll(sharedDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sharedDir, tf.TerraformLockFile), []byte(""), 0o644))

	lockPath := filepath.Join(s.dir, tf.TerraformLockFile)
	require.NoError(t, os.Remove(lockPath))
	require.NoError(t, os.Symlink(filepath.Join("..", "shared-lock", tf.TerraformLockFile), lockPath))

	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

	opts := newNoDotTerragruntCacheOpts(t, s, "init")
	cfg := &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true}}

	require.NoError(t, run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()))

	calls := rec.snapshot()
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0].Args, "init")

	info, err := os.Lstat(lockPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "%s must still be a symlink after an in-place init", lockPath)
}

func TestRunNoDotTerragruntCacheNullInputIsWrittenForTheRun(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	nullVarsPath := filepath.Join(s.dir, run.NullTFVarsFile)

	var seenDuringRun atomic.Bool

	exec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		_, err := os.Stat(filepath.Join(inv.Dir, run.NullTFVarsFile))
		seenDuringRun.Store(err == nil)

		return vexec.Result{}
	})
	v := venvtest.New().WithExec(exec).WithFS(vfs.NewOSFS())

	opts := newNoDotTerragruntCacheOpts(t, s, "plan")
	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true},
		Inputs:    map[string]any{"var1": nil, "var2": "value"},
	}

	require.NoError(t, run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter()))

	assert.True(t, seenDuringRun.Load(), "the null-vars file must exist in the unit directory while tofu runs")
	assert.NoFileExists(t, nullVarsPath, "the null-vars file must be removed after the run")
}

func TestRunNoDotTerragruntCacheNullInputKeepsExistingFile(t *testing.T) {
	t.Parallel()

	// Anything already at the reserved path in the unit directory must survive the run untouched, because the
	// null-vars file is removed afterwards. Terragrunt cannot tell a user file from its own leftover, so it stops.
	testCases := []struct {
		name     string
		contents string
		symlink  bool
	}{
		{name: "file with values", contents: `{"keep": "original"}`},
		{name: "file with only null values", contents: `{"var1": null}`},
		{name: "symlink to another file", contents: `{"var1": null}`, symlink: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := setupRunE2EScaffold(t)
			nullVarsPath := filepath.Join(s.dir, run.NullTFVarsFile)
			targetPath := nullVarsPath

			if tc.symlink {
				targetPath = filepath.Join(s.dir, "null-vars-target.json")
				require.NoError(t, os.Symlink(filepath.Base(targetPath), nullVarsPath))
			}

			require.NoError(t, os.WriteFile(targetPath, []byte(tc.contents), 0o644))

			rec := &recorder{}
			v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())

			opts := newNoDotTerragruntCacheOpts(t, s, "plan")
			cfg := &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{NoDotTerragruntCache: true},
				Inputs:    map[string]any{"var1": nil},
			}

			err := run.Run(t.Context(), logger.CreateLogger(), v, opts, report.NewReport(), cfg, creds.NewGetter())

			var exists run.NullVarsFileExists

			require.ErrorAs(t, err, &exists)
			assert.Equal(t, nullVarsPath, exists.Path)
			assert.Empty(t, rec.snapshot(), "tofu must not run")

			contents, err := os.ReadFile(targetPath)
			require.NoError(t, err)
			assert.Equal(t, tc.contents, string(contents))

			info, err := os.Lstat(nullVarsPath)
			require.NoError(t, err)
			assert.Equal(t, tc.symlink, info.Mode()&os.ModeSymlink != 0, "symlink state of %s changed", nullVarsPath)
		})
	}
}

// TestRunNullInputReplacesLeftoverCacheFile pins the default cache path: a null-vars file left in the cache
// working directory, for example by an interrupted run, is disposable and is replaced on the next run.
func TestRunNullInputReplacesLeftoverCacheFile(t *testing.T) {
	t.Parallel()

	s := setupRunE2EScaffold(t)
	rec := &recorder{}
	v := venvtest.New().WithExec(vexec.NewMemExec(rec.handler(vexec.Result{}))).WithFS(vfs.NewOSFS())
	l := logger.CreateLogger()

	opts := newRunE2EOpts(t, s, "plan")
	cfg := &runcfg.RunConfig{Inputs: map[string]any{"var1": nil}}

	require.NoError(t, run.Run(t.Context(), l, v, opts, report.NewReport(), cfg, creds.NewGetter()))

	cacheMainFiles, err := filepath.Glob(filepath.Join(s.dir, ".terragrunt-cache", "*", "*", "main.tf"))
	require.NoError(t, err)
	require.Len(t, cacheMainFiles, 1, "expected exactly one cache working directory")

	leftover := filepath.Join(filepath.Dir(cacheMainFiles[0]), run.NullTFVarsFile)
	require.NoError(t, os.WriteFile(leftover, []byte(`{"var1":`), 0o600))

	require.NoError(t, run.Run(t.Context(), l, v, opts, report.NewReport(), cfg, creds.NewGetter()))

	assert.NoFileExists(t, leftover, "a leftover null-vars file in the cache must be replaced and removed")
}

// newNoDotTerragruntCacheOpts returns scaffold options with the no-dot-terragrunt-cache experiment enabled.
func newNoDotTerragruntCacheOpts(t *testing.T, s runE2EScaffold, command string) *run.Options {
	t.Helper()

	opts := newRunE2EOpts(t, s, command)
	require.NoError(t, opts.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))

	return opts
}

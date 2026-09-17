package config_test

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateStackCASOfflineMissFails pins that --cas-offline turns a unit
// source the store lacks into a generation error rather than a fallback to
// the standard getter, which would fetch over the network the flag forbids.
func TestGenerateStackCASOfflineMissFails(t *testing.T) {
	t.Parallel()

	srv, err := git.NewServer()
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, srv.Close()) })

	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# unit\n"), "init"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	stackDir := filepath.Join(tmpDir, "stack")
	stackPath := filepath.Join(stackDir, "terragrunt.stack.hcl")

	// The store must be empty for the miss, so the CAS is pointed at a
	// cache directory of this test's own rather than the machine's.
	v := venvtest.NewOSWithEmptyEnv()
	platform := *v.Platform
	platform.UserCacheDir = func() (string, error) { return filepath.Join(tmpDir, "cache"), nil }
	v.Platform = &platform

	body := `unit "vpc" {
  source = "git::` + repoURL + `?ref=main"
  path   = "vpc"
}
`

	require.NoError(t, v.FS.MkdirAll(stackDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, stackPath, []byte(body), 0o644))

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	_, pctx := config.NewParsingContext(t.Context(), l, v, config.WithStrictControls(controls.New()))
	pctx.TerragruntStackConfigPath = stackPath
	pctx.RootWorkingDir = stackDir
	pctx.WorkingDir = stackDir
	pctx.TerragruntConfigPath = stackPath
	pctx.CASCloneDepth = 1
	pctx.CASOffline = true
	pctx.Experiments = experiment.NewExperiments()

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	// Components are generated on the pool, so the fetch error surfaces from Wait.
	genErr := config.GenerateStackFile(t.Context(), l, pctx, pool, stackPath)
	waitErr := pool.Wait()

	require.ErrorIs(t, errors.Join(genErr, waitErr), cas.ErrCASOffline)

	assert.NoFileExists(t, filepath.Join(stackDir, config.StackDir, "vpc", "main.tf"),
		"the standard getter must not have fetched the unit")
}

// TestGenerateStackCASOfflineSetupFailureFails pins that --cas-offline stops
// stack generation when the CAS cannot be built. Disabling CAS features would
// send every remote component to the standard getter, over the network the
// flag forbids.
func TestGenerateStackCASOfflineSetupFailureFails(t *testing.T) {
	t.Parallel()

	errNoCacheDir := errors.New("no cache dir")

	srv, err := git.NewServer()
	require.NoError(t, err)

	t.Cleanup(func() { assert.NoError(t, srv.Close()) })

	require.NoError(t, srv.CommitFile(t.Context(), "main.tf", []byte("# unit\n"), "init"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	stackDir := filepath.Join(tmpDir, "stack")
	stackPath := filepath.Join(stackDir, "terragrunt.stack.hcl")

	// A cache directory that cannot be resolved is what makes cas.New fail.
	v := venvtest.NewOSWithEmptyEnv()
	platform := *v.Platform
	platform.UserCacheDir = func() (string, error) { return "", errNoCacheDir }
	v.Platform = &platform

	body := `unit "vpc" {
  source = "git::` + repoURL + `?ref=main"
  path   = "vpc"
}
`

	require.NoError(t, v.FS.MkdirAll(stackDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, stackPath, []byte(body), 0o644))

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	_, pctx := config.NewParsingContext(t.Context(), l, v, config.WithStrictControls(controls.New()))
	pctx.TerragruntStackConfigPath = stackPath
	pctx.RootWorkingDir = stackDir
	pctx.WorkingDir = stackDir
	pctx.TerragruntConfigPath = stackPath
	pctx.CASCloneDepth = 1
	pctx.CASOffline = true
	pctx.Experiments = experiment.NewExperiments()

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	genErr := config.GenerateStackFile(t.Context(), l, pctx, pool, stackPath)
	waitErr := pool.Wait()

	require.ErrorIs(t, errors.Join(genErr, waitErr), errNoCacheDir)

	assert.NoFileExists(t, filepath.Join(stackDir, config.StackDir, "vpc", "main.tf"),
		"the standard getter must not have fetched the unit")
}

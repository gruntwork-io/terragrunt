package runnerpool_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestBuildCanonicalConfigPath_DirectoryPath(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	unit := component.NewUnit(tmpDir)

	canonicalPath, canonicalDir, err := runnerpool.BuildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, config.DefaultTerragruntConfigPath), canonicalPath)
	assert.Equal(t, tmpDir, canonicalDir)
	assert.Equal(t, tmpDir, unit.Path())
}

func TestBuildCanonicalConfigPath_HCLSuffix(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	unit := component.NewUnit(configPath)

	canonicalPath, canonicalDir, err := runnerpool.BuildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, configPath, canonicalPath)
	assert.Equal(t, tmpDir, canonicalDir)
}

func TestBuildCanonicalConfigPath_JSONSuffix(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl.json")
	unit := component.NewUnit(configPath)

	canonicalPath, canonicalDir, err := runnerpool.BuildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, configPath, canonicalPath)
	assert.Equal(t, tmpDir, canonicalDir)
}

func TestBuildCanonicalConfigPath_RelativePath(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	unit := component.NewUnit("subdir")

	canonicalPath, canonicalDir, err := runnerpool.BuildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(subDir, config.DefaultTerragruntConfigPath), canonicalPath)
	assert.Equal(t, subDir, canonicalDir)
	assert.Equal(t, subDir, unit.Path())
}

func TestCloneUnitOptions_NilStackExecution(t *testing.T) {
	t.Parallel()

	stack := component.NewStack(helpers.TmpDirWOSymlinks(t))
	unit := component.NewUnit("/some/path")
	l := thlogger.CreateLogger()

	opts, logger, err := runnerpool.CloneUnitOptions(stack, unit, "/some/path/terragrunt.hcl", "", l)

	require.NoError(t, err)
	assert.Nil(t, opts)
	assert.NotNil(t, logger)
}

func TestCloneUnitOptions_WithStackExecution(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "stack", "terragrunt.hcl"))
	require.NoError(t, err)

	stack := component.NewStack(tmpDir)
	stack.Execution = &component.StackExecution{
		TerragruntOptions: stackOpts,
	}

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	opts, logger, err := runnerpool.CloneUnitOptions(stack, unit, configPath, "", l)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.NotNil(t, logger)
	assert.Equal(t, configPath, opts.OriginalTerragruntConfigPath)
	assert.NotEmpty(t, opts.DownloadDir)
}

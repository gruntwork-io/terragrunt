package runnerpool_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/runner/runnerpool"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestCloneUnitOptions_WithStackOpts(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "stack", "terragrunt.hcl"))
	require.NoError(t, err)

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	opts, logger, err := runnerpool.CloneUnitOptions(stackOpts, unit, configPath, "", l)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.NotNil(t, logger)
	assert.Equal(t, configPath, opts.OriginalTerragruntConfigPath)
	assert.NotEmpty(t, opts.DownloadDir)
}

func TestBuildUnitOpts_BasicUnit(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	unitOpts, unitLogger, err := runnerpool.BuildUnitOpts(l, stackOpts, unit)

	require.NoError(t, err)
	require.NotNil(t, unitOpts)
	assert.NotNil(t, unitLogger)
	assert.Contains(t, unitOpts.TerragruntConfigPath, "terragrunt.hcl")
}

func TestBuildUnitOpts_WithDiscoveryContext(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "terragrunt.hcl"))
	require.NoError(t, err)

	unit := component.NewUnit(tmpDir)
	unit.SetDiscoveryContext(&component.DiscoveryContext{
		Cmd:  "plan",
		Args: []string{"-input=false"},
	})

	l := thlogger.CreateLogger()

	unitOpts, _, err := runnerpool.BuildUnitOpts(l, stackOpts, unit)

	require.NoError(t, err)
	assert.Equal(t, "plan", unitOpts.TerraformCommand)
}

func TestCloneUnitOptions_WithCustomDownloadDir(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "stack", "terragrunt.hcl"))
	require.NoError(t, err)

	stackOpts.DownloadDir = "/custom/download"

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	opts, _, err := runnerpool.CloneUnitOptions(stackOpts, unit, configPath, "", l)

	require.NoError(t, err)
	assert.Equal(t, "/custom/download", opts.DownloadDir)
}

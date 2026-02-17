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

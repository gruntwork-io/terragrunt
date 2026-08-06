package runnerpool_test

import (
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

func TestCloneUnitOptions_WithStackOpts(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(tmpDir, "stack", "terragrunt.hcl"),
	)
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

func TestBuildUnitOpts_WithSource(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	moduleSource := "git::ssh://git@github.com/acme/modules.git//vpc"
	badSource := "no-slashes-here"

	testCases := []struct {
		unitConfig     *config.TerragruntConfig
		name           string
		stackSource    string
		expectedSource string
		expectedErr    string
	}{
		{
			name:        "no stack source",
			unitConfig:  &config.TerragruntConfig{},
			stackSource: "",
		},
		{
			name:           "unit has no parsed config inherits the stack source",
			stackSource:    "/local/modules",
			expectedSource: "/local/modules",
		},
		{
			name:           "unit has no terraform source inherits the stack source",
			unitConfig:     &config.TerragruntConfig{},
			stackSource:    "/local/modules",
			expectedSource: "/local/modules",
		},
		{
			name: "per-unit source is computed",
			unitConfig: &config.TerragruntConfig{
				Terraform: &config.TerraformConfig{Source: &moduleSource},
			},
			stackSource:    "/local/modules",
			expectedSource: "/local/modules//vpc",
		},
		{
			name: "unparsable source errors",
			unitConfig: &config.TerragruntConfig{
				Terraform: &config.TerraformConfig{Source: &badSource},
			},
			stackSource: "/local/modules",
			expectedErr: "failed to compute source for unit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stackOpts, err := options.NewTerragruntOptionsForTest(
				filepath.Join(tmpDir, "terragrunt.hcl"),
			)
			require.NoError(t, err)

			stackOpts.Source = tc.stackSource

			unit := component.NewUnit(tmpDir)
			if tc.unitConfig != nil {
				unit = unit.WithConfig(tc.unitConfig)
			}

			unitOpts, _, err := runnerpool.BuildUnitOpts(thlogger.CreateLogger(), stackOpts, unit)

			if tc.expectedErr != "" {
				require.ErrorContains(t, err, tc.expectedErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.expectedSource, unitOpts.Source)
		})
	}
}

func TestBuildUnitOpts_UnitPathIsAConfigFile(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(configPath)
	require.NoError(t, err)

	unitOpts, _, err := runnerpool.BuildUnitOpts(
		thlogger.CreateLogger(),
		stackOpts,
		component.NewUnit(configPath),
	)
	require.NoError(t, err)
	assert.Equal(t, configPath, unitOpts.TerragruntConfigPath)
}

func TestCloneUnitOptions_WithCustomDownloadDir(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(
		filepath.Join(tmpDir, "stack", "terragrunt.hcl"),
	)
	require.NoError(t, err)

	stackOpts.DownloadDir = "/custom/download"

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	opts, _, err := runnerpool.CloneUnitOptions(stackOpts, unit, configPath, "", l)

	require.NoError(t, err)
	assert.Equal(t, "/custom/download", opts.DownloadDir)
}

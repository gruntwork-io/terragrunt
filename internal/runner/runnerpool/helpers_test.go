//nolint:testpackage // Internal tests for unexported helper functions
package runnerpool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/options"
	thlogger "github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestBuildCanonicalConfigPath_DirectoryPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	unit := component.NewUnit(tmpDir)

	canonicalPath, canonicalDir, err := buildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, config.DefaultTerragruntConfigPath), canonicalPath)
	assert.Equal(t, tmpDir, canonicalDir)
	assert.Equal(t, tmpDir, unit.Path())
}

func TestBuildCanonicalConfigPath_HCLSuffix(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	unit := component.NewUnit(configPath)

	canonicalPath, canonicalDir, err := buildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, configPath, canonicalPath)
	assert.Equal(t, tmpDir, canonicalDir)
}

func TestBuildCanonicalConfigPath_JSONSuffix(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "terragrunt.hcl.json")
	unit := component.NewUnit(configPath)

	canonicalPath, canonicalDir, err := buildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, configPath, canonicalPath)
	assert.Equal(t, tmpDir, canonicalDir)
}

func TestBuildCanonicalConfigPath_RelativePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0o755))

	unit := component.NewUnit("subdir")

	canonicalPath, canonicalDir, err := buildCanonicalConfigPath(unit, tmpDir)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(subDir, config.DefaultTerragruntConfigPath), canonicalPath)
	assert.Equal(t, subDir, canonicalDir)
	assert.Equal(t, subDir, unit.Path())
}

func TestCloneUnitOptions_NilStackExecution(t *testing.T) {
	t.Parallel()

	stack := component.NewStack(t.TempDir())
	unit := component.NewUnit("/some/path")
	l := thlogger.CreateLogger()

	opts, logger, err := cloneUnitOptions(stack, unit, "/some/path/terragrunt.hcl", "", l)

	require.NoError(t, err)
	assert.Nil(t, opts)
	assert.NotNil(t, logger)
}

func TestCloneUnitOptions_WithStackExecution(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")

	stackOpts, err := options.NewTerragruntOptionsForTest(filepath.Join(tmpDir, "stack", "terragrunt.hcl"))
	require.NoError(t, err)

	stack := component.NewStack(tmpDir)
	stack.Execution = &component.StackExecution{
		TerragruntOptions: stackOpts,
	}

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	opts, logger, err := cloneUnitOptions(stack, unit, configPath, "", l)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.NotNil(t, logger)
	assert.Equal(t, configPath, opts.OriginalTerragruntConfigPath)
	assert.NotEmpty(t, opts.DownloadDir)
}

func TestShouldSkipUnitWithoutTerraform_WithSource(t *testing.T) {
	t.Parallel()

	source := "github.com/example/module"
	cfg := &config.TerragruntConfig{
		Terraform: &config.TerraformConfig{
			Source: &source,
		},
	}
	unit := component.NewUnit(t.TempDir()).WithConfig(cfg)
	l := thlogger.CreateLogger()

	skip, err := shouldSkipUnitWithoutTerraform(unit, t.TempDir(), l)

	require.NoError(t, err)
	assert.False(t, skip)
}

func TestShouldSkipUnitWithoutTerraform_WithTFFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(""), 0o600))

	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	skip, err := shouldSkipUnitWithoutTerraform(unit, tmpDir, l)

	require.NoError(t, err)
	assert.False(t, skip)
}

func TestShouldSkipUnitWithoutTerraform_NoSourceNoFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	unit := component.NewUnit(tmpDir)
	l := thlogger.CreateLogger()

	skip, err := shouldSkipUnitWithoutTerraform(unit, tmpDir, l)

	require.NoError(t, err)
	assert.True(t, skip)
}

func TestShouldSkipUnitWithoutTerraform_EmptySource(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	source := ""
	cfg := &config.TerragruntConfig{
		Terraform: &config.TerraformConfig{
			Source: &source,
		},
	}
	unit := component.NewUnit(tmpDir).WithConfig(cfg)
	l := thlogger.CreateLogger()

	skip, err := shouldSkipUnitWithoutTerraform(unit, tmpDir, l)

	require.NoError(t, err)
	assert.True(t, skip)
}

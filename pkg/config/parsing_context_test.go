package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithDependencyConfigPath_CustomDownloadDir_Preserved verifies that when the user
// has set a custom TG_DOWNLOAD_DIR, switching to a dependency's config path via
// WithDependencyConfigPath preserves the custom directory unchanged.
//
// This is the core contract that prevents the regression where external dependency
// .terragrunt-cache directories were created at the dependency's local path instead
// of inside the user-configured TG_DOWNLOAD_DIR.
func TestWithDependencyConfigPath_CustomDownloadDir_Preserved(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	callerConfigPath := filepath.Join(tmpDir, "modules", "app", "terragrunt.hcl")
	depConfigPath := filepath.Join(tmpDir, "modules", "vpc", "terragrunt.hcl")
	customDownloadDir := filepath.Join(tmpDir, "custom-cache")

	l := logger.CreateLogger()
	_, pctx := config.NewParsingContext(t.Context(), l, config.WithStrictControls(controls.New()))

	_, callerDefaultDir := util.DefaultWorkingAndDownloadDirs(callerConfigPath)
	pctx.TerragruntConfigPath = callerConfigPath
	pctx.DownloadDir = customDownloadDir // simulates TG_DOWNLOAD_DIR=/custom-cache

	// Sanity: the custom dir must differ from the caller's default.
	require.NotEqual(t, callerDefaultDir, customDownloadDir)

	_, depCtx, err := pctx.WithDependencyConfigPath(l, depConfigPath)
	require.NoError(t, err)

	assert.Equal(t, customDownloadDir, depCtx.DownloadDir,
		"custom TG_DOWNLOAD_DIR must be preserved across WithDependencyConfigPath")
	assert.Equal(t, depConfigPath, depCtx.TerragruntConfigPath)
}

// TestWithDependencyConfigPath_DefaultDownloadDir_Updated verifies that when no custom
// TG_DOWNLOAD_DIR is set (the caller uses its own default .terragrunt-cache),
// WithDependencyConfigPath updates DownloadDir to the dependency's own default.
func TestWithDependencyConfigPath_DefaultDownloadDir_Updated(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	callerConfigPath := filepath.Join(tmpDir, "modules", "app", "terragrunt.hcl")
	depConfigPath := filepath.Join(tmpDir, "modules", "vpc", "terragrunt.hcl")

	l := logger.CreateLogger()
	_, pctx := config.NewParsingContext(t.Context(), l, config.WithStrictControls(controls.New()))

	// Set DownloadDir to the caller's default (no TG_DOWNLOAD_DIR override).
	_, callerDefaultDir := util.DefaultWorkingAndDownloadDirs(callerConfigPath)
	pctx.TerragruntConfigPath = callerConfigPath
	pctx.DownloadDir = callerDefaultDir

	_, depCtx, err := pctx.WithDependencyConfigPath(l, depConfigPath)
	require.NoError(t, err)

	_, expectedDepDownloadDir := util.DefaultWorkingAndDownloadDirs(depConfigPath)
	assert.Equal(t, expectedDepDownloadDir, depCtx.DownloadDir,
		"DownloadDir should be updated to the dependency's default when no custom TG_DOWNLOAD_DIR is set")
}

// TestWithDependencyConfigPath_CustomDownloadDir_NotDefaultForAnyModule verifies that a
// custom TG_DOWNLOAD_DIR that happens to share a prefix with module paths is still
// preserved correctly (no accidental substring matching).
func TestWithDependencyConfigPath_CustomDownloadDir_NotDefaultForAnyModule(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	callerConfigPath := filepath.Join(tmpDir, "app", "terragrunt.hcl")
	depConfigPath := filepath.Join(tmpDir, "dep", "terragrunt.hcl")

	// A custom path that shares the tmpDir root but is not a module-default cache.
	customDownloadDir := filepath.Join(tmpDir, ".terragrunt-cache")

	l := logger.CreateLogger()
	_, pctx := config.NewParsingContext(t.Context(), l, config.WithStrictControls(controls.New()))

	_, callerDefaultDir := util.DefaultWorkingAndDownloadDirs(callerConfigPath)
	pctx.TerragruntConfigPath = callerConfigPath
	pctx.DownloadDir = customDownloadDir

	// Sanity: the custom dir must differ from the caller's per-module default.
	require.NotEqual(t, callerDefaultDir, customDownloadDir)

	_, depCtx, err := pctx.WithDependencyConfigPath(l, depConfigPath)
	require.NoError(t, err)

	assert.Equal(t, customDownloadDir, depCtx.DownloadDir,
		"custom TG_DOWNLOAD_DIR must be preserved even when it shares the root tmpDir")
}

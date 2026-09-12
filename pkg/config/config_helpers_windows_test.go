//go:build windows

package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/require"
)

// TestPathRelativeToIncludeWindowsSeparators pins that path_relative_to_include
// returns forward-slash paths even though filepath.Rel produces native (backslash)
// separators on Windows. The result is frequently embedded in a remote-state key
// (e.g. the azurerm backend builds its blob URL from it), where a backslash is
// rejected as an invalid control character. Gated to Windows because
// filepath.ToSlash is a no-op on Unix separators.
func TestPathRelativeToIncludeWindowsSeparators(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(
		helpers.RootFolder,
		"child",
		"sub-child",
		config.DefaultTerragruntConfigPath,
	)
	trackInclude := getTrackIncludeFromTestData(
		map[string]config.IncludeConfig{
			"": {Path: filepath.Join(helpers.RootFolder, config.DefaultTerragruntConfigPath)},
		},
		nil,
		configPath,
	)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), configPath)
	pctx = pctx.WithTrackInclude(trackInclude)

	actualPath, err := config.PathRelativeToInclude(ctx, pctx, l, nil)
	require.NoError(t, err)
	require.Equal(t, "child/sub-child", actualPath)
}

// TestPathRelativeFromIncludeWindowsSeparators mirrors the above for
// path_relative_from_include.
func TestPathRelativeFromIncludeWindowsSeparators(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(
		helpers.RootFolder,
		"child",
		config.DefaultTerragruntConfigPath,
	)
	trackInclude := getTrackIncludeFromTestData(
		map[string]config.IncludeConfig{
			"": {Path: filepath.Join(helpers.RootFolder, "sibling", config.DefaultTerragruntConfigPath)},
		},
		nil,
		configPath,
	)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), configPath)
	pctx = pctx.WithTrackInclude(trackInclude)

	actualPath, err := config.PathRelativeFromInclude(ctx, pctx, l, nil)
	require.NoError(t, err)
	require.Equal(t, "../child", actualPath)
}

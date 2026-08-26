//go:build tf

package test_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFMarkManyAsReadRelpathSourceTriggersDiscovery(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureMarkManyAsReadRelpath)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMarkManyAsReadRelpath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMarkManyAsReadRelpath)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive "+
			"--queue-include-units-reading=modules/foo/main.tf --report-file %s "+
			"--working-dir %s -- plan",
		helpers.ReportFile, rootPath,
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)

	assert.NotContains(t, stdout+stderr, "No units discovered",
		"unit should be discovered via the local module source walk")

	runs, err := report.ParseJSONRunsFromFile(vfs.NewOSFS(), filepath.Join(rootPath, helpers.ReportFile))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"live/unit"}, runs.Names())
}

// TestTFMarkManyAsReadDefaultDiscoversModuleChanges pins that the local module
// source walk is on by default: a reading= filter naming a module file selects
// the unit consuming that module, with no experiment flag.
func TestTFMarkManyAsReadDefaultDiscoversModuleChanges(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureMarkManyAsReadRelpath)

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMarkManyAsReadRelpath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMarkManyAsReadRelpath)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	cmd := fmt.Sprintf(
		"terragrunt run --all --non-interactive "+
			"--filter 'reading=modules/foo/main.tf' --report-file %s "+
			"--working-dir %s -- plan",
		helpers.ReportFile, rootPath,
	)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)

	runs, err := report.ParseJSONRunsFromFile(vfs.NewOSFS(), filepath.Join(rootPath, helpers.ReportFile))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"live/unit"}, runs.Names(),
		"module source changes should cascade to the unit by default")
}

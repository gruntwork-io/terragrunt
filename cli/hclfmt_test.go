package cli

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/options"
)

func TestHCLFmt(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	expected, err := ioutil.ReadFile("../test/fixture-hclfmt/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	tgOptions.WorkingDir = tmpPath

	err = runHCLFmt(tgOptions)
	require.NoError(t, err)

	dirs := []string{
		"terragrunt.hcl",
		"a/terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
	}
	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		dir := dir

		// Create a synchronous subtest to group the child tests so that they can run in parallel while honoring cleanup
		// routines in the main test.
		t.Run("group", func(t *testing.T) {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				tgHclPath := filepath.Join(tmpPath, dir)
				actual, err := ioutil.ReadFile(tgHclPath)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
			})
		})
	}
}

func TestHCLFmtErrors(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt-errors", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	dirs := []string{
		"dangling-attribute",
		"invalid-character",
		"invalid-key",
	}
	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		dir := dir

		// Create a synchronous subtest to group the child tests so that they can run in parallel while honoring cleanup
		// routines in the main test.
		t.Run("group", func(t *testing.T) {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				tgHclDir := filepath.Join(tmpPath, dir)
				newTgOptions := tgOptions.Clone(tgOptions.TerragruntConfigPath)
				newTgOptions.WorkingDir = tgHclDir
				err := runHCLFmt(tgOptions)
				require.Error(t, err)
			})
		})
	}
}

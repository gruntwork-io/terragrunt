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
	"github.com/gruntwork-io/terragrunt/util"
)

func TestHCLFmt(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	expected, err := util.ReadFileAsString("../test/fixture-hclfmt/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	tgOptions.WorkingDir = tmpPath

	err = runHCLFmt(tgOptions)
	require.NoError(t, err)

	t.Run("group", func(t *testing.T) {
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
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				tgHclPath := filepath.Join(tmpPath, dir)
				actual, err := util.ReadFileAsString(tgHclPath)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
			})
		}

		// Finally, check to make sure the file in the `.terragrunt-cache` folder was ignored and untouched
		t.Run("terragrunt-cache", func(t *testing.T) {
			t.Parallel()

			originalTgHclPath := "../test/fixture-hclfmt/ignored/.terragrunt-cache/terragrunt.hcl"
			original, err := util.ReadFileAsString(originalTgHclPath)
			require.NoError(t, err)

			tgHclPath := filepath.Join(tmpPath, "ignored/.terragrunt-cache/terragrunt.hcl")
			actual, err := util.ReadFileAsString(tgHclPath)
			require.NoError(t, err)

			assert.Equal(t, original, actual)
		})
	})

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
				err := runHCLFmt(newTgOptions)
				require.Error(t, err)
			})
		})
	}
}

func TestHCLFmtCheck(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt-check", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	expected, err := ioutil.ReadFile("../test/fixture-hclfmt-check/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	tgOptions.Check = true
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

func TestHCLFmtCheckErrors(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt-check-errors", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	expected, err := ioutil.ReadFile("../test/fixture-hclfmt-check-errors/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)
	tgOptions.Check = true
	tgOptions.WorkingDir = tmpPath

	err = runHCLFmt(tgOptions)
	require.Error(t, err)

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

func TestHCLFmtFile(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt-file", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	expected, err := ioutil.ReadFile("../test/fixture-hclfmt-file/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
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
				newTgOptions := tgOptions.Clone(tgOptions.TerragruntConfigPath)
				newTgOptions.HclFile = dir
				newTgOptions.WorkingDir = tmpPath
				err = runHCLFmt(newTgOptions)
				require.NoError(t, err)

				actual, err := ioutil.ReadFile(tgHclPath)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
			})
		})
	}
}

func TestHCLFmtFileErrors(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../test/fixture-hclfmt-file-errors", t.Name(), func(path string) bool { return true })
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

				newTgOptions := tgOptions.Clone(tgOptions.TerragruntConfigPath)
				newTgOptions.HclFile = dir
				newTgOptions.WorkingDir = tmpPath
				err = runHCLFmt(newTgOptions)
				require.Error(t, err)
			})
		})
	}

}

package hclfmt_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terratest/modules/files"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/cli/commands/hclfmt"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

func TestHCLFmt(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("./testdata/fixtures", t.Name(), func(path string) bool { return true })

	t.Cleanup(func() {
		os.RemoveAll(tmpPath)
	})

	require.NoError(t, err)

	expected, err := util.ReadFileAsString("./testdata/fixtures/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath
	tgOptions.HclExclude = []string{".history"}

	err = hclfmt.Run(tgOptions)
	require.NoError(t, err)

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		dirs := []string{
			"terragrunt.hcl",
			"a/terragrunt.hcl",
			"a/b/c/terragrunt.hcl",
			"a/b/c/d/services.hcl",
			"a/b/c/d/e/terragrunt.hcl",
		}
		for _, dir := range dirs {
			// Capture range variable into for block so it doesn't change while looping
			dir := dir

			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				tgHclPath := filepath.Join(tmpPath, dir)
				actual, err := util.ReadFileAsString(tgHclPath)
				require.NoError(t, err)
				assert.Equal(t, expected, actual)
			})
		}

		// check to make sure the file in the `.terragrunt-cache` folder was ignored and untouched
		t.Run("terragrunt-cache", func(t *testing.T) {
			t.Parallel()

			originalTgHclPath := "./testdata/fixtures/ignored/.terragrunt-cache/terragrunt.hcl"
			original, err := util.ReadFileAsString(originalTgHclPath)
			require.NoError(t, err)

			tgHclPath := filepath.Join(tmpPath, "ignored/.terragrunt-cache/terragrunt.hcl")
			actual, err := util.ReadFileAsString(tgHclPath)
			require.NoError(t, err)

			assert.Equal(t, original, actual)
		})

		// Finally, check to make sure the file in the `.history` folder was ignored and untouched
		t.Run("history", func(t *testing.T) {
			t.Parallel()

			originalTgHclPath := "./testdata/fixtures/ignored/.history/terragrunt.hcl"
			original, err := util.ReadFileAsString(originalTgHclPath)
			require.NoError(t, err)

			tgHclPath := filepath.Join(tmpPath, "ignored/.history/terragrunt.hcl")
			actual, err := util.ReadFileAsString(tgHclPath)
			require.NoError(t, err)

			assert.Equal(t, original, actual)
		})
	})

}

func TestHCLFmtErrors(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../../../test/fixtures/hclfmt-errors", t.Name(), func(path string) bool { return true })
	t.Cleanup(func() {
		os.RemoveAll(tmpPath)
	})
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

		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			tgHclDir := filepath.Join(tmpPath, dir)
			newTgOptions, err := tgOptions.Clone(tgOptions.TerragruntConfigPath)
			require.NoError(t, err)

			newTgOptions.WorkingDir = tgHclDir

			err = hclfmt.Run(newTgOptions)
			require.Error(t, err)
		})
	}
}

func TestHCLFmtCheck(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../../../test/fixtures/hclfmt-check", t.Name(), func(path string) bool { return true })

	t.Cleanup(func() {
		os.RemoveAll(tmpPath)
	})

	require.NoError(t, err)

	expected, err := os.ReadFile("../../../test/fixtures/hclfmt-check/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.Check = true
	tgOptions.WorkingDir = tmpPath

	err = hclfmt.Run(tgOptions)
	require.NoError(t, err)

	dirs := []string{
		"terragrunt.hcl",
		"a/terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
		"a/b/c/d/services.hcl",
		"a/b/c/d/e/terragrunt.hcl",
	}

	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		dir := dir

		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			tgHclPath := filepath.Join(tmpPath, dir)
			actual, err := os.ReadFile(tgHclPath)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestHCLFmtCheckErrors(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../../../test/fixtures/hclfmt-check-errors", t.Name(), func(path string) bool { return true })

	t.Cleanup(func() {
		os.RemoveAll(tmpPath)
	})

	require.NoError(t, err)

	expected, err := os.ReadFile("../../../test/fixtures/hclfmt-check-errors/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.Check = true
	tgOptions.WorkingDir = tmpPath

	err = hclfmt.Run(tgOptions)
	require.Error(t, err)

	dirs := []string{
		"terragrunt.hcl",
		"a/terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
		"a/b/c/d/services.hcl",
		"a/b/c/d/e/terragrunt.hcl",
	}

	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		dir := dir

		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			tgHclPath := filepath.Join(tmpPath, dir)
			actual, err := os.ReadFile(tgHclPath)
			require.NoError(t, err)
			assert.Equal(t, expected, actual)
		})
	}
}

func TestHCLFmtFile(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("./testdata/fixtures", t.Name(), func(path string) bool { return true })

	t.Cleanup(func() {
		os.RemoveAll(tmpPath)
	})

	require.NoError(t, err)

	expected, err := os.ReadFile("./testdata/fixtures/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// format only the hcl file contained within the a subdirectory of the fixture
	tgOptions.HclFile = "a/terragrunt.hcl"
	tgOptions.WorkingDir = tmpPath
	err = hclfmt.Run(tgOptions)
	require.NoError(t, err)

	// test that the formatting worked on the specified file
	t.Run("formatted", func(t *testing.T) {
		t.Run(tgOptions.HclFile, func(t *testing.T) {
			t.Parallel()
			tgHclPath := filepath.Join(tmpPath, tgOptions.HclFile)
			formatted, err := os.ReadFile(tgHclPath)
			require.NoError(t, err)
			assert.Equal(t, expected, formatted)
		})
	})

	dirs := []string{
		"terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
	}

	original, err := os.ReadFile("./testdata/fixtures/terragrunt.hcl")
	require.NoError(t, err)

	// test that none of the other files were formatted
	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		dir := dir

		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			testingPath := filepath.Join(tmpPath, dir)
			actual, err := os.ReadFile(testingPath)
			require.NoError(t, err)
			assert.Equal(t, original, actual)
		})
	}
}

func TestHCLFmtStdin(t *testing.T) {
	t.Parallel()

	realStdin := os.Stdin
	realStdout := os.Stdout

	tempStdoutFile, err := os.CreateTemp(t.TempDir(), "stdout.hcl")
	defer func() {
		_ = tempStdoutFile.Close()
	}()
	require.NoError(t, err)

	os.Stdout = tempStdoutFile
	defer func() { os.Stdout = realStdout }()

	os.Stdin, err = os.Open("../../../test/fixtures/hclfmt-stdin/terragrunt.hcl")
	defer func() { os.Stdin = realStdin }()
	require.NoError(t, err)

	expected, err := os.ReadFile("../../../test/fixtures/hclfmt-stdin/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// format hcl from stdin
	tgOptions.HclFromStdin = true
	err = hclfmt.Run(tgOptions)
	require.NoError(t, err)

	formatted, err := os.ReadFile(tempStdoutFile.Name())
	require.NoError(t, err)
	assert.Equal(t, expected, formatted)
}

func TestHCLFmtHeredoc(t *testing.T) {
	t.Parallel()

	tmpPath, err := files.CopyFolderToTemp("../../../test/fixtures/hclfmt-heredoc", t.Name(), func(path string) bool { return true })
	defer os.RemoveAll(tmpPath)
	require.NoError(t, err)

	expected, err := os.ReadFile("../../../test/fixtures/hclfmt-heredoc/expected.hcl")
	require.NoError(t, err)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath

	err = hclfmt.Run(tgOptions)
	require.NoError(t, err)

	tgHclPath := filepath.Join(tmpPath, "terragrunt.hcl")
	actual, err := os.ReadFile(tgHclPath)
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
}

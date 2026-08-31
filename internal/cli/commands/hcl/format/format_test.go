package format_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// readFixture reads a fixture-relative path out of the in-memory filesystem.
func readFixture(t *testing.T, fsys vfs.FS, root string, elem ...string) string {
	t.Helper()

	contents, err := vfs.ReadFile(fsys, filepath.Join(append([]string{root}, elem...)...))
	require.NoError(t, err)

	return string(contents)
}

// onDisk reads a file straight from the repository, for the unformatted and
// expected forms the fixtures ship with.
func onDisk(t *testing.T, path string) string {
	t.Helper()

	contents, err := vfs.ReadFile(vfs.NewOSFS(), path)
	require.NoError(t, err)

	return string(contents)
}

func TestHCLFmt(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "./testdata/fixtures")

	expected := onDisk(t, "./testdata/fixtures/expected.hcl")
	original := onDisk(t, "./testdata/fixtures/terragrunt.hcl")

	// .gitignore covers the cache directory, so a fixture cannot carry this file.
	cached := filepath.Join(tmpPath, "ignored", util.TerragruntCacheDir, "terragrunt.hcl")
	require.NoError(t, vfs.WriteFile(fsys, cached, []byte(original), 0o644))

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath
	tgOptions.HclExclude = []string{".history"}

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
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
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, expected, readFixture(t, fsys, tmpPath, dir))
			})
		}

		// Formatting a cached copy edits a file the next download overwrites.
		t.Run("terragrunt-cache", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, original,
				readFixture(t, fsys, tmpPath, "ignored", util.TerragruntCacheDir, "terragrunt.hcl"))
		})

		// Finally, check to make sure the file in the `.history` folder was ignored and untouched
		t.Run("history", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t,
				onDisk(t, "./testdata/fixtures/ignored/.history/terragrunt.hcl"),
				readFixture(t, fsys, tmpPath, "ignored/.history/terragrunt.hcl"))
		})
	})
}

func TestHCLFmtErrors(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "../../../../../test/fixtures/hclfmt-errors")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	dirs := []string{
		"dangling-attribute",
		"invalid-character",
		"invalid-key",
	}
	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			tgHclDir := filepath.Join(tmpPath, dir)
			l, newTgOptions, err := tgOptions.CloneWithConfigPath(
				logger.CreateLogger(),
				tgOptions.TerragruntConfigPath,
			)
			require.NoError(t, err)

			newTgOptions.WorkingDir = tgHclDir

			err = format.Run(t.Context(), l, venvtest.New().WithFS(fsys), newTgOptions)
			require.Error(t, err)
		})
	}
}

func TestHCLFmtCheck(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "../../../../../test/fixtures/hclfmt-check")

	expected := onDisk(t, "../../../../../test/fixtures/hclfmt-check/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.Check = true
	tgOptions.WorkingDir = tmpPath

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
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
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, readFixture(t, fsys, tmpPath, dir))
		})
	}
}

func TestHCLFmtCheckErrors(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "../../../../../test/fixtures/hclfmt-check-errors")

	expected := onDisk(t, "../../../../../test/fixtures/hclfmt-check-errors/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.Check = true
	tgOptions.WorkingDir = tmpPath

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
	require.Error(t, err)

	dirs := []string{
		"terragrunt.hcl",
		"a/terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
		"a/b/c/d/services.hcl",
		"a/b/c/d/e/terragrunt.hcl",
	}

	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, readFixture(t, fsys, tmpPath, dir))
		})
	}
}

func TestHCLFmtFile(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "./testdata/fixtures")

	expected := onDisk(t, "./testdata/fixtures/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// format only the hcl file contained within the a subdirectory of the fixture
	tgOptions.HclFile = "a/terragrunt.hcl"
	tgOptions.WorkingDir = tmpPath
	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
	require.NoError(t, err)

	// test that the formatting worked on the specified file
	t.Run("formatted", func(t *testing.T) {
		t.Run(tgOptions.HclFile, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, expected, readFixture(t, fsys, tmpPath, tgOptions.HclFile))
		})
	})

	dirs := []string{
		"terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
	}

	original := onDisk(t, "./testdata/fixtures/terragrunt.hcl")

	// test that none of the other files were formatted
	for _, dir := range dirs {
		// Capture range variable into for block so it doesn't change while looping
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, original, readFixture(t, fsys, tmpPath, dir))
		})
	}
}

func TestHCLFmtStdin(t *testing.T) {
	t.Parallel()

	unformatted := onDisk(t, "../../../../../test/fixtures/hclfmt-stdin/terragrunt.hcl")
	expected := onDisk(t, "../../../../../test/fixtures/hclfmt-stdin/expected.hcl")

	tests := map[string]struct {
		input               string
		wantStdout          string
		wantDiffLines       []string
		check               bool
		diff                bool
		wantNeedsFormatting bool
	}{
		"formatted content goes to stdout": {
			input:      unformatted,
			wantStdout: expected,
		},
		"check reports unformatted input": {
			input:               unformatted,
			check:               true,
			wantNeedsFormatting: true,
		},
		"check accepts formatted input": {
			input: expected,
			check: true,
		},
		"diff shows what would change": {
			input:         unformatted,
			diff:          true,
			wantDiffLines: []string{"--- old/stdin", "+++ new/stdin", "+  foo = \"bar\""},
		},
		"diff is empty for formatted input": {
			input: expected,
			diff:  true,
		},
		"check and diff both apply": {
			input:               unformatted,
			check:               true,
			diff:                true,
			wantDiffLines:       []string{"--- old/stdin", "+++ new/stdin"},
			wantNeedsFormatting: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tgOptions, err := options.NewTerragruntOptionsForTest("")
			require.NoError(t, err)

			tgOptions.HclFromStdin = true
			tgOptions.Check = tc.check
			tgOptions.Diff = tc.diff

			var out bytes.Buffer

			v := venvtest.New().
				WithStdin(strings.NewReader(tc.input)).
				WithWriter(&out)

			err = format.Run(t.Context(), logger.CreateLogger(), v, tgOptions)

			stdout := out.String()

			for _, want := range tc.wantDiffLines {
				assert.Contains(t, stdout, want)
			}

			if len(tc.wantDiffLines) > 0 {
				assert.NotContains(t, stdout, expected, "--diff prints the diff instead of the formatted content")
			}

			if len(tc.wantDiffLines) == 0 {
				assert.Equal(t, tc.wantStdout, stdout)
			}

			if !tc.wantNeedsFormatting {
				require.NoError(t, err)

				return
			}

			var needsFormatting *format.FileNeedsFormattingError

			require.ErrorAs(t, err, &needsFormatting)
			assert.Equal(t, "stdin", needsFormatting.Path)
		})
	}
}

func TestHCLFmtHeredoc(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "../../../../../test/fixtures/hclfmt-heredoc")

	expected := onDisk(t, "../../../../../test/fixtures/hclfmt-heredoc/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
	require.NoError(t, err)

	assert.Equal(t, expected, readFixture(t, fsys, tmpPath, "terragrunt.hcl"))
}

func TestRunForFiles(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "./testdata/fixtures")

	expected := onDisk(t, filepath.Join(".", "testdata", "fixtures", "expected.hcl"))
	original := onDisk(t, filepath.Join(".", "testdata", "fixtures", "terragrunt.hcl"))

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Format only specific files, mixing relative and absolute paths, including a non-hcl file
	absPath := filepath.Join(tmpPath, "a", "b", "c", "terragrunt.hcl")
	files := []string{
		"terragrunt.hcl", // relative, should be formatted
		absPath,          // absolute, should be formatted
		filepath.Join("a", "b", "c", "d", "services.hcl"), // relative, should be formatted
		filepath.Join("a", "terragrunt.hcl"),              // relative, should be formatted
		"README.md",                                       // non-hcl, should be skipped
	}

	err = format.RunForFiles(
		t.Context(),
		logger.CreateLogger(),
		venvtest.New().WithFS(fsys),
		tgOptions,
		tmpPath,
		files,
	)
	require.NoError(t, err)

	// Verify formatted files
	for _, rel := range []string{
		"terragrunt.hcl",
		filepath.Join("a", "b", "c", "terragrunt.hcl"),
		filepath.Join("a", "b", "c", "d", "services.hcl"),
		filepath.Join("a", "terragrunt.hcl"),
	} {
		assert.Equal(t, expected, readFixture(t, fsys, tmpPath, rel),
			"File %s should be formatted", rel)
	}

	// Verify file NOT in the list was left untouched
	assert.Equal(t, original,
		readFixture(t, fsys, tmpPath, "a", "b", "c", "d", "e", "terragrunt.hcl"),
		"File a/b/c/d/e/terragrunt.hcl should NOT be formatted")
}

func TestRunForFilesEmptyList(t *testing.T) {
	t.Parallel()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	err = format.RunForFiles(
		t.Context(),
		logger.CreateLogger(),
		venvtest.New(),
		tgOptions,
		"/empty",
		nil,
	)
	require.NoError(t, err)
}

func TestHCLFmtFilter(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "./testdata/fixtures")

	expected := onDisk(t, "./testdata/fixtures/expected.hcl")

	original := onDisk(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	err = tgOptions.Experiments.EnableExperiment("filter-flag")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath

	filters, parseErr := filter.ParseFilterQueries(logger.CreateLogger(), []string{"./a/b/**"})
	require.NoError(t, parseErr)

	tgOptions.Filters = filters

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
	require.NoError(t, err)

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		formattedDirs := []string{
			"a/b/c/terragrunt.hcl",
			"a/b/c/d/services.hcl",
			"a/b/c/d/e/terragrunt.hcl",
		}
		for _, dir := range formattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, expected, readFixture(t, fsys, tmpPath, dir), "File %s should be formatted", dir)
			})
		}

		unformattedDirs := []string{
			"terragrunt.hcl",
			"a/terragrunt.hcl",
		}
		for _, dir := range unformattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, original, readFixture(t, fsys, tmpPath, dir), "File %s should NOT be formatted", dir)
			})
		}
	})
}

func TestHCLFmtFilterMultiple(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "./testdata/fixtures")

	expected := onDisk(t, "./testdata/fixtures/expected.hcl")

	original := onDisk(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	err = tgOptions.Experiments.EnableExperiment("filter-flag")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath

	filters, parseErr := filter.ParseFilterQueries(logger.CreateLogger(), []string{
		filepath.Join(tmpPath, "terragrunt.hcl"),
		"./a/b/c/d/e/**",
	})
	require.NoError(t, parseErr)

	tgOptions.Filters = filters

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
	require.NoError(t, err)

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		formattedDirs := []string{
			"terragrunt.hcl",
			"a/b/c/d/e/terragrunt.hcl",
		}
		for _, dir := range formattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, expected, readFixture(t, fsys, tmpPath, dir), "File %s should be formatted", dir)
			})
		}

		unformattedDirs := []string{
			"a/terragrunt.hcl",
			"a/b/c/terragrunt.hcl",
			"a/b/c/d/services.hcl",
		}
		for _, dir := range unformattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, original, readFixture(t, fsys, tmpPath, dir), "File %s should NOT be formatted", dir)
			})
		}
	})
}

func TestHCLFmtFilterNegation(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := venvtest.LoadFS(t, "./testdata/fixtures")

	expected := onDisk(t, "./testdata/fixtures/expected.hcl")

	original := onDisk(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	err = tgOptions.Experiments.EnableExperiment("filter-flag")
	require.NoError(t, err)

	tgOptions.WorkingDir = tmpPath

	filters, parseErr := filter.ParseFilterQueries(logger.CreateLogger(), []string{
		"./a/**",
		"!./a/b/c/d/**",
	})
	require.NoError(t, parseErr)

	tgOptions.Filters = filters

	err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
	require.NoError(t, err)

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		formattedDirs := []string{
			"a/terragrunt.hcl",
			"a/b/c/terragrunt.hcl",
		}
		for _, dir := range formattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, expected, readFixture(t, fsys, tmpPath, dir), "File %s should be formatted", dir)
			})
		}

		unformattedDirs := []string{
			"terragrunt.hcl",
			"a/b/c/d/services.hcl",
			"a/b/c/d/e/terragrunt.hcl",
		}
		for _, dir := range unformattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()

				assert.Equal(t, original, readFixture(t, fsys, tmpPath, dir), "File %s should NOT be formatted", dir)
			})
		}
	})
}

// TestHCLFmtDiffFile pins what --diff prints for a file on the filesystem.
// [TestHCLFmtStdin] covers the same flag for content arriving on standard
// input, where the header names stdin instead of a path.
//
// The header names the path the file was found at. An in-memory root gives
// the same string on every machine, so this compares the header too.
func TestHCLFmtDiffFile(t *testing.T) {
	t.Parallel()

	const fixture = "../../../../../test/fixtures/hclfmt-diff"

	fsys, root := venvtest.LoadFS(t, fixture)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = root
	tgOptions.Diff = true

	var out bytes.Buffer

	require.NoError(t, format.Run(
		t.Context(),
		logger.CreateLogger(),
		venvtest.New().WithFS(fsys).WithWriter(&out),
		tgOptions,
	))

	formatted := filepath.Join(root, "terragrunt.hcl")
	header := fmt.Sprintf(
		"diff old%[1]s new%[1]s\n--- old%[1]s\n+++ new%[1]s\n",
		formatted,
	)

	assert.Equal(t, header+onDisk(t, filepath.Join(fixture, "expected.diff")), out.String())
}

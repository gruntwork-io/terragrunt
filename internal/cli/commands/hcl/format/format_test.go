package format_test

import (
	"bytes"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// loadFixture copies the on-disk fixture tree at dir into a fresh in-memory
// filesystem and returns it with the root the copy landed at. The tests run
// the format command against that filesystem, so a formatter that reached for
// os instead of the venv would leave the fixture untouched and fail loudly.
//
// Only files are copied: writing one registers its parent directories, so the
// tree arrives with the copy. An empty fixture directory would not survive,
// and none of these fixtures has one.
func loadFixture(t *testing.T, dir string) (vfs.FS, string) {
	t.Helper()

	const root = "/fixture"

	src, dst := vfs.NewOSFS(), vfs.NewMemMapFS()
	abs := helpers.MustAbs(t, dir)

	require.NoError(t, vfs.WalkDir(src, abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		contents, err := vfs.ReadFile(src, path)
		if err != nil {
			return err
		}

		return vfs.WriteFile(dst, filepath.Join(root, rel), contents, info.Mode())
	}))

	return dst, root
}

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

	fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

	expected := onDisk(t, "./testdata/fixtures/expected.hcl")

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

		// check to make sure the file in the `.terragrunt-cache` folder was ignored and untouched
		t.Run("terragrunt-cache", func(t *testing.T) {
			t.Parallel()

			assert.Equal(t,
				onDisk(t, "./testdata/fixtures/ignored/.terragrunt-cache/terragrunt.hcl"),
				readFixture(t, fsys, tmpPath, "ignored/.terragrunt-cache/terragrunt.hcl"))
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

	fsys, tmpPath := loadFixture(t, "../../../../../test/fixtures/hclfmt-errors")

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

	fsys, tmpPath := loadFixture(t, "../../../../../test/fixtures/hclfmt-check")

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

	fsys, tmpPath := loadFixture(t, "../../../../../test/fixtures/hclfmt-check-errors")

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

	fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

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

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	var formatted bytes.Buffer

	// format hcl from stdin
	tgOptions.HclFromStdin = true

	v := venvtest.New().
		WithStdin(strings.NewReader(unformatted)).
		WithWriter(&formatted)

	err = format.Run(t.Context(), logger.CreateLogger(), v, tgOptions)
	require.NoError(t, err)

	assert.Equal(t, expected, formatted.String())
}

func TestHCLFmtHeredoc(t *testing.T) {
	t.Parallel()

	fsys, tmpPath := loadFixture(t, "../../../../../test/fixtures/hclfmt-heredoc")

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

	fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

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

	fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

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

	fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

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

	fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

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

func TestHCLFmtExcludeDirPath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		excludes []string
		untouched []string
		formatted []string
	}{
		{
			name:      "relative path",
			excludes:  []string{"a/b/c"},
			untouched: []string{"a/b/c/terragrunt.hcl", "a/b/c/d/services.hcl", "a/b/c/d/e/terragrunt.hcl"},
			formatted: []string{"terragrunt.hcl", "a/terragrunt.hcl"},
		},
		{
			name:      "normalized path",
			excludes:  []string{"./a/b/c/"},
			untouched: []string{"a/b/c/terragrunt.hcl"},
			formatted: []string{"a/terragrunt.hcl"},
		},
		{
			name:      "doublestar pattern",
			excludes:  []string{"**/c"},
			untouched: []string{"a/b/c/terragrunt.hcl", "a/b/c/d/services.hcl"},
			formatted: []string{"a/terragrunt.hcl"},
		},
		{
			name:      "basename still works",
			excludes:  []string{"c"},
			untouched: []string{"a/b/c/terragrunt.hcl"},
			formatted: []string{"a/terragrunt.hcl"},
		},
	}

	unformatted := onDisk(t, "./testdata/fixtures/terragrunt.hcl")
	expected := onDisk(t, "./testdata/fixtures/expected.hcl")

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			fsys, tmpPath := loadFixture(t, "./testdata/fixtures")

			tgOptions, err := options.NewTerragruntOptionsForTest("")
			require.NoError(t, err)

			tgOptions.WorkingDir = tmpPath
			tgOptions.HclExclude = c.excludes

			err = format.Run(t.Context(), logger.CreateLogger(), venvtest.New().WithFS(fsys), tgOptions)
			require.NoError(t, err)

			for _, path := range c.untouched {
				assert.Equal(t, unformatted, readFixture(t, fsys, tmpPath, filepath.FromSlash(path)),
					"%s must stay untouched", path)
			}
			for _, path := range c.formatted {
				assert.Equal(t, expected, readFixture(t, fsys, tmpPath, filepath.FromSlash(path)),
					"%s must be formatted", path)
			}
		})
	}
}

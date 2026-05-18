package format_test

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/hcl/format"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

const memWorkDir = "/work"

func TestHCLFmt(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "./testdata/fixtures", memWorkDir)

	expected := readDiskAsString(t, "./testdata/fixtures/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = memWorkDir
	tgOptions.HclExclude = []string{".history"}

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

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
			t.Run(dir, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)))
			})
		}

		t.Run("terragrunt-cache", func(t *testing.T) {
			t.Parallel()
			original := readDiskAsString(t, "./testdata/fixtures/ignored/.terragrunt-cache/terragrunt.hcl")
			actual := readMemFSAsString(t, fsys, filepath.Join(memWorkDir, "ignored/.terragrunt-cache/terragrunt.hcl"))
			assert.Equal(t, original, actual)
		})

		t.Run("history", func(t *testing.T) {
			t.Parallel()
			original := readDiskAsString(t, "./testdata/fixtures/ignored/.history/terragrunt.hcl")
			actual := readMemFSAsString(t, fsys, filepath.Join(memWorkDir, "ignored/.history/terragrunt.hcl"))
			assert.Equal(t, original, actual)
		})
	})
}

func TestHCLFmtErrors(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "../../../../../test/fixtures/hclfmt-errors", memWorkDir)

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	dirs := []string{
		"dangling-attribute",
		"invalid-character",
		"invalid-key",
	}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()

			l, newTgOptions, err := tgOptions.CloneWithConfigPath(logger.CreateLogger(), tgOptions.TerragruntConfigPath)
			require.NoError(t, err)

			newTgOptions.WorkingDir = filepath.Join(memWorkDir, dir)

			require.Error(t, format.Run(t.Context(), l, v, newTgOptions))
		})
	}
}

func TestHCLFmtCheck(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "../../../../../test/fixtures/hclfmt-check", memWorkDir)

	expected := readDiskAsString(t, "../../../../../test/fixtures/hclfmt-check/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.Check = true
	tgOptions.WorkingDir = memWorkDir

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

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
			assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)))
		})
	}
}

func TestHCLFmtCheckErrors(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "../../../../../test/fixtures/hclfmt-check-errors", memWorkDir)

	expected := readDiskAsString(t, "../../../../../test/fixtures/hclfmt-check-errors/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.Check = true
	tgOptions.WorkingDir = memWorkDir

	require.Error(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

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
			assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)))
		})
	}
}

func TestHCLFmtFile(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "./testdata/fixtures", memWorkDir)

	expected := readDiskAsString(t, "./testdata/fixtures/expected.hcl")
	original := readDiskAsString(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.HclFile = "a/terragrunt.hcl"
	tgOptions.WorkingDir = memWorkDir

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

	t.Run("formatted", func(t *testing.T) {
		t.Run(tgOptions.HclFile, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, tgOptions.HclFile)))
		})
	})

	dirs := []string{
		"terragrunt.hcl",
		"a/b/c/terragrunt.hcl",
	}
	for _, dir := range dirs {
		t.Run(dir, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, original, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)))
		})
	}
}

// TestHCLFmtStdin exercises the --stdin path. stdin/stdout are process-level
// IO, not filesystem, so this test legitimately swaps os.Stdin and os.Stdout
// rather than using a MemMapFS.
func TestHCLFmtStdin(t *testing.T) {
	t.Parallel()

	realStdin := os.Stdin
	realStdout := os.Stdout

	tempStdoutFile, err := os.CreateTemp(helpers.TmpDirWOSymlinks(t), "stdout.hcl")
	require.NoError(t, err)

	defer func() { _ = tempStdoutFile.Close() }()

	os.Stdout = tempStdoutFile

	defer func() { os.Stdout = realStdout }()

	os.Stdin, err = os.Open("../../../../../test/fixtures/hclfmt-stdin/terragrunt.hcl")
	require.NoError(t, err)

	defer func() { os.Stdin = realStdin }()

	expected := readDiskAsString(t, "../../../../../test/fixtures/hclfmt-stdin/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.HclFromStdin = true

	v := venv.OSVenv()
	v.Writers = &writer.Writers{Writer: os.Stdout, ErrWriter: io.Discard}

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

	formatted := readDiskAsString(t, tempStdoutFile.Name())
	assert.Equal(t, expected, formatted)
}

func TestHCLFmtHeredoc(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "../../../../../test/fixtures/hclfmt-heredoc", memWorkDir)

	expected := readDiskAsString(t, "../../../../../test/fixtures/hclfmt-heredoc/expected.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	tgOptions.WorkingDir = memWorkDir

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

	assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, "terragrunt.hcl")))
}

func TestRunForFiles(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "./testdata/fixtures", memWorkDir)

	expected := readDiskAsString(t, "./testdata/fixtures/expected.hcl")
	original := readDiskAsString(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	absPath := filepath.Join(memWorkDir, "a", "b", "c", "terragrunt.hcl")
	files := []string{
		"terragrunt.hcl",
		absPath,
		filepath.Join("a", "b", "c", "d", "services.hcl"),
		filepath.Join("a", "terragrunt.hcl"),
		"README.md",
	}

	require.NoError(t, format.RunForFiles(t.Context(), logger.CreateLogger(), v, tgOptions, memWorkDir, files))

	for _, rel := range []string{
		"terragrunt.hcl",
		filepath.Join("a", "b", "c", "terragrunt.hcl"),
		filepath.Join("a", "b", "c", "d", "services.hcl"),
		filepath.Join("a", "terragrunt.hcl"),
	} {
		assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, rel)),
			"File %s should be formatted", rel)
	}

	assert.Equal(t, original, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, "a", "b", "c", "d", "e", "terragrunt.hcl")),
		"File a/b/c/d/e/terragrunt.hcl should NOT be formatted")
}

func TestRunForFilesEmptyList(t *testing.T) {
	t.Parallel()

	v, _ := newMemVenv()

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	require.NoError(t, format.RunForFiles(t.Context(), logger.CreateLogger(), v, tgOptions, memWorkDir, nil))
}

func TestHCLFmtFilter(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "./testdata/fixtures", memWorkDir)

	expected := readDiskAsString(t, "./testdata/fixtures/expected.hcl")
	original := readDiskAsString(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	require.NoError(t, tgOptions.Experiments.EnableExperiment("filter-flag"))

	tgOptions.WorkingDir = memWorkDir

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"./a/b/**"})
	require.NoError(t, err)

	tgOptions.Filters = filters

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

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
				assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)),
					"File %s should be formatted", dir)
			})
		}

		unformattedDirs := []string{
			"terragrunt.hcl",
			"a/terragrunt.hcl",
		}
		for _, dir := range unformattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, original, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)),
					"File %s should NOT be formatted", dir)
			})
		}
	})
}

func TestHCLFmtFilterMultiple(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "./testdata/fixtures", memWorkDir)

	expected := readDiskAsString(t, "./testdata/fixtures/expected.hcl")
	original := readDiskAsString(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	require.NoError(t, tgOptions.Experiments.EnableExperiment("filter-flag"))

	tgOptions.WorkingDir = memWorkDir

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{
		filepath.Join(memWorkDir, "terragrunt.hcl"),
		"./a/b/c/d/e/**",
	})
	require.NoError(t, err)

	tgOptions.Filters = filters

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		formattedDirs := []string{
			"terragrunt.hcl",
			"a/b/c/d/e/terragrunt.hcl",
		}
		for _, dir := range formattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)),
					"File %s should be formatted", dir)
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
				assert.Equal(t, original, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)),
					"File %s should NOT be formatted", dir)
			})
		}
	})
}

func TestHCLFmtFilterNegation(t *testing.T) {
	t.Parallel()

	v, fsys := newMemVenv()
	seedMemFSFromDisk(t, fsys, "./testdata/fixtures", memWorkDir)

	expected := readDiskAsString(t, "./testdata/fixtures/expected.hcl")
	original := readDiskAsString(t, "./testdata/fixtures/terragrunt.hcl")

	tgOptions, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	require.NoError(t, tgOptions.Experiments.EnableExperiment("filter-flag"))

	tgOptions.WorkingDir = memWorkDir

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{
		"./a/**",
		"!./a/b/c/d/**",
	})
	require.NoError(t, err)

	tgOptions.Filters = filters

	require.NoError(t, format.Run(t.Context(), logger.CreateLogger(), v, tgOptions))

	t.Run("group", func(t *testing.T) {
		t.Parallel()

		formattedDirs := []string{
			"a/terragrunt.hcl",
			"a/b/c/terragrunt.hcl",
		}
		for _, dir := range formattedDirs {
			t.Run(dir, func(t *testing.T) {
				t.Parallel()
				assert.Equal(t, expected, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)),
					"File %s should be formatted", dir)
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
				assert.Equal(t, original, readMemFSAsString(t, fsys, filepath.Join(memWorkDir, dir)),
					"File %s should NOT be formatted", dir)
			})
		}
	})
}

// seedMemFSFromDisk walks srcDir on the real filesystem and copies every entry
// into fsys at dstRoot/<rel>. Tests use this to stage fixtures on an in-memory
// filesystem so format.Run can operate without touching the real disk.
func seedMemFSFromDisk(t *testing.T, fsys vfs.FS, srcDir, dstRoot string) {
	t.Helper()

	require.NoError(t, filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		dst := filepath.Join(dstRoot, rel)

		if d.IsDir() {
			return fsys.MkdirAll(dst, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		return vfs.WriteFile(fsys, dst, data, 0o644)
	}))
}

// newMemVenv builds a venv backed by a fresh MemMapFS with stdout/stderr
// captured to io.Discard so format command output doesn't pollute test logs.
func newMemVenv() (*venv.Venv, vfs.FS) {
	fsys := vfs.NewMemMapFS()

	return &venv.Venv{
		FS:      fsys,
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}, fsys
}

func readDiskAsString(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

func readMemFSAsString(t *testing.T, fsys vfs.FS, path string) string {
	t.Helper()

	b, err := vfs.ReadFile(fsys, path)
	require.NoError(t, err)

	return string(b)
}

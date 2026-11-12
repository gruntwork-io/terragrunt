package patch_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/patch"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// baseDir is the directory the functions resolve relative paths against. It
	// exists only in the in-memory filesystem each test builds, so a function
	// reaching the real disk finds nothing there.
	baseDir = "/fixture/unit"

	// recursionDepthEnv is the variable the patched templatefile reads its
	// recursion cap from.
	recursionDepthEnv = "TF_TEMPLATE_RECURSION_DEPTH"
)

// memVenv returns a venv whose filesystem holds files, each path relative to
// [baseDir].
func memVenv(t *testing.T, files map[string]string) *venv.Venv {
	t.Helper()

	return venv.MemVenv().WithFS(venvtest.NewFS(t, baseDir, files))
}

func TestFileReadsThroughTheVenv(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"user-data.sh": "#!/bin/sh\necho hi\n"})

	got, err := patch.FileFunc(v, baseDir, false, noRead).
		Call([]cty.Value{cty.StringVal("user-data.sh")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("#!/bin/sh\necho hi\n"), got)
}

func TestFileBase64EncodesContents(t *testing.T) {
	t.Parallel()

	const contents = "\xff\xfe binary"

	v := memVenv(t, map[string]string{"blob.bin": contents})

	got, err := patch.FileFunc(v, baseDir, true, noRead).
		Call([]cty.Value{cty.StringVal("blob.bin")})
	require.NoError(t, err)
	assert.Equal(
		t,
		cty.StringVal(base64.StdEncoding.EncodeToString([]byte(contents))),
		got,
	)
}

func TestFileRejectsContentsThatAreNotUTF8(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"blob.bin": "\xff\xfe binary"})

	_, err := patch.FileFunc(v, baseDir, false, noRead).
		Call([]cty.Value{cty.StringVal("blob.bin")})
	require.Error(t, err)
}

func TestFileFailsWhenTheFileIsMissing(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"present.txt": "here"})

	_, err := patch.FileFunc(v, baseDir, false, noRead).
		Call([]cty.Value{cty.StringVal("absent.txt")})
	require.Error(t, err)
}

func TestFileLeavesARelativeBaseDirToTheFilesystem(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	require.NoError(
		t,
		vfs.WriteFile(fsys, "data.txt", []byte("beside the root\n"), 0o644),
	)

	v := venv.MemVenv().WithFS(fsys)
	v.Platform.Getwd = func() (string, error) { return "/elsewhere", nil }

	got, err := patch.FileFunc(v, ".", false, noRead).
		Call([]cty.Value{cty.StringVal("data.txt")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("beside the root\n"), got)
}

func TestFileExistsReportsWhatTheVenvHolds(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"present.txt": "here"})

	fn := patch.FileExistsFunc(v, baseDir, noRead)

	got, err := fn.Call([]cty.Value{cty.StringVal("present.txt")})
	require.NoError(t, err)
	assert.Equal(t, cty.True, got)

	got, err = fn.Call([]cty.Value{cty.StringVal("absent.txt")})
	require.NoError(t, err)
	assert.Equal(t, cty.False, got)
}

func TestFileExistsFailsOnADirectory(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"modules/main.tf": "# module"})

	_, err := patch.FileExistsFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("modules")})
	require.Error(t, err)
}

func TestFileSetMatchesRelativePaths(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{
		"main.tf":            "# root",
		"README.md":          "# docs",
		"modules/a/main.tf":  "# a",
		"modules/b/vars.tf":  "# b",
		"modules/b/notes.md": "# notes",
	})

	fn := patch.FileSetFunc(v, baseDir, noRead)

	got, err := fn.Call([]cty.Value{cty.StringVal("."), cty.StringVal("*.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.SetVal([]cty.Value{cty.StringVal("main.tf")}), got)

	got, err = fn.Call(
		[]cty.Value{cty.StringVal("."), cty.StringVal("modules/**/*.tf")},
	)
	require.NoError(t, err)
	assert.Equal(t, cty.SetVal([]cty.Value{
		cty.StringVal("modules/a/main.tf"),
		cty.StringVal("modules/b/vars.tf"),
	}), got)
}

func TestFileSetReturnsAnEmptySetWhenNothingMatches(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"main.tf": "# root"})

	got, err := patch.FileSetFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal("*.json")})
	require.NoError(t, err)
	assert.Equal(t, cty.SetValEmpty(cty.String), got)
}

var fileSetTree = map[string]string{
	"main.tf":              "# root",
	"resources/a.json":     "{}",
	"resources/sub/b.json": "{}",
	"br[1]/d.json":         "{}",
	"../shared/c.json":     "{}",
}

func TestFileSetMatchesPatterns(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		path    string
		pattern string
		want    []string
	}{
		{
			name:    "leading dot slash",
			path:    "./",
			pattern: "./resources/*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "leading dot slash with double star",
			path:    ".",
			pattern: "./resources/**/*.json",
			want:    []string{"resources/a.json", "resources/sub/b.json"},
		},
		{
			name:    "leading dot slash without a glob",
			path:    ".",
			pattern: "./resources/a.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "leading dot slash under a sub directory",
			path:    "./resources",
			pattern: "./*.json",
			want:    []string{"a.json"},
		},
		{
			name:    "parent back into the path",
			path:    "./resources",
			pattern: "../resources/*.json",
			want:    []string{"a.json"},
		},
		{
			name:    "inner dot",
			path:    ".",
			pattern: "resources/./*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "double slash",
			path:    ".",
			pattern: "resources//*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "inner parent",
			path:    ".",
			pattern: "resources/sub/../*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "leading slash",
			path:    ".",
			pattern: "/resources/*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "above the path",
			path:    ".",
			pattern: "../shared/*.json",
			want:    []string{"../shared/c.json"},
		},
		{
			name:    "two levels above the path",
			path:    "./resources",
			pattern: "../../shared/*.json",
			want:    []string{"../../shared/c.json"},
		},
		{
			name:    "canonical pattern",
			path:    ".",
			pattern: "resources/**/*.json",
			want:    []string{"resources/a.json", "resources/sub/b.json"},
		},
		{
			name:    "one directory deep",
			path:    ".",
			pattern: "./*/*.json",
			want:    []string{"br[1]/d.json", "resources/a.json"},
		},
		{
			name:    "double star from the path",
			path:    ".",
			pattern: "./**/*.json",
			want:    []string{"br[1]/d.json", "resources/a.json", "resources/sub/b.json"},
		},
		{
			name:    "alternatives that form a double star",
			path:    ".",
			pattern: "./{*,}*/*.json",
			want:    []string{"br[1]/d.json", "resources/a.json", "resources/sub/b.json"},
		},
		{
			name:    "dot after alternatives",
			path:    ".",
			pattern: "{resources,x}/./*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "two directories deep",
			path:    ".",
			pattern: "*/*/*.json",
			want:    []string{"resources/sub/b.json"},
		},
		{
			name:    "alternatives of different depths",
			path:    ".",
			pattern: "./{main.tf,resources/sub/*.json}",
			want:    []string{"main.tf", "resources/sub/b.json"},
		},
		{
			name:    "trailing slash on the pattern",
			path:    ".",
			pattern: "resources/**/",
			want:    []string{"resources/a.json", "resources/sub/b.json"},
		},
		{
			name:    "trailing slash on the path",
			path:    "./resources/",
			pattern: "*.json",
			want:    []string{"a.json"},
		},
		{
			name:    "empty pattern",
			path:    ".",
			pattern: "",
		},
		{
			name:    "dot pattern",
			path:    ".",
			pattern: ".",
		},
		{
			name:    "dot slash pattern",
			path:    ".",
			pattern: "./",
		},
		{
			name:    "parent pattern",
			path:    ".",
			pattern: "..",
		},
		{
			name:    "pattern back to the path",
			path:    ".",
			pattern: "resources/..",
		},
		{
			name:    "glob under a file",
			path:    ".",
			pattern: "resources/a.json/*",
		},
		{
			name:    "glob under a file after dot slash",
			path:    ".",
			pattern: "./resources/a.json/*",
		},
		{
			name:    "dot under a file path",
			path:    "main.tf",
			pattern: ".",
			want:    []string{"."},
		},
		{
			name:    "missing directory above the path",
			path:    ".",
			pattern: "../missing/*.json",
		},
		{
			name:    "backslash is not a separator",
			path:    ".",
			pattern: `.\resources\*.json`,
		},
		{
			name:    "escaped letter in a directory name",
			path:    ".",
			pattern: `./resources/su\b/*.json`,
			want:    []string{"resources/sub/b.json"},
		},
		{
			name:    "escaped literal prefix",
			path:    ".",
			pattern: `./br\[1\]/*.json`,
			want:    []string{"br[1]/d.json"},
		},
		{
			name:    "escaped bracket before a parent",
			path:    ".",
			pattern: `resources/x\[y]/../*.json`,
			want:    []string{"resources/a.json"},
		},
		{
			name:    "escaped brace before a parent",
			path:    ".",
			pattern: `resources/x\{y\}/../*.json`,
			want:    []string{"resources/a.json"},
		},
		{
			name:    "glob under a file path",
			path:    "main.tf",
			pattern: "*",
		},
		{
			name:    "parent back to a file path",
			path:    "main.tf",
			pattern: "../main.tf",
			want:    []string{"."},
		},
		{
			name:    "glob under a file path reached through its parent",
			path:    "main.tf",
			pattern: "../main.tf/*",
		},
		{
			name:    "leading slash before a parent",
			path:    ".",
			pattern: "/../shared/*.json",
			want:    []string{"../shared/c.json"},
		},
		{
			name:    "question mark in the first element",
			path:    ".",
			pattern: "./?esources/*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "unclosed brace that cleaning removes",
			path:    ".",
			pattern: "{x/../resources/*.json",
			want:    []string{"resources/a.json"},
		},
		{
			name:    "dot inside alternatives",
			path:    ".",
			pattern: "{resources,resources/./sub}/*.json",
			want:    []string{"resources/a.json", "resources/sub/b.json"},
		},
		{
			name:    "parent inside alternatives",
			path:    ".",
			pattern: "{resources,y/../c}/*.json",
		},
		{
			name:    "parent inside a class",
			path:    ".",
			pattern: "[r/../q]esources/*.json",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(t, fileSetTree)

			got, err := patch.FileSetFunc(v, baseDir, noRead).
				Call([]cty.Value{cty.StringVal(tc.path), cty.StringVal(tc.pattern)})
			require.NoError(t, err)
			assert.Equal(t, stringSet(tc.want), got)
		})
	}
}

func TestFileSetRejectsAMalformedPattern(t *testing.T) {
	t.Parallel()

	for _, path := range []string{".", "missing"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			v := memVenv(t, fileSetTree)

			_, err := patch.FileSetFunc(v, baseDir, noRead).
				Call([]cty.Value{cty.StringVal(path), cty.StringVal("./resources/[")})
			require.ErrorIs(t, err, glob.ErrBadPattern)
		})
	}
}

func TestFileSetReportsTheCleanPathOfEachMatch(t *testing.T) {
	t.Parallel()

	v := memVenv(t, fileSetTree)

	var reads []string

	_, err := patch.FileSetFunc(v, baseDir, func(path string) { reads = append(reads, path) }).
		Call([]cty.Value{cty.StringVal("./resources"), cty.StringVal("./../../shared/*.json")})
	require.NoError(t, err)
	assert.Equal(t, []string{filepath.Join(baseDir, "..", "shared", "c.json")}, reads)
}

func TestFileSetKeepsAnEscapeBeforeTheFirstWildcard(t *testing.T) {
	t.Parallel()

	v := memVenv(t, fileSetTree)

	got, err := patch.FileSetFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal(`re\sources/*.json`)})
	require.NoError(t, err)
	assert.Equal(t, stringSet([]string{"resources/a.json"}), got)
}

func TestFileSetReturnsNothingWhenCleaningStrandsAnEscape(t *testing.T) {
	t.Parallel()

	v := memVenv(t, fileSetTree)

	got, err := patch.FileSetFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal(`resources\/`)})
	require.NoError(t, err)
	assert.Equal(t, cty.SetValEmpty(cty.String), got)
}

func TestFileSetSkipsAnUnreadableDirectory(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{
		"resources/a.json":        "{}",
		"resources/locked/b.json": "{}",
	})
	v.FS = &openFailFS{FS: v.FS, dir: filepath.Join(baseDir, "resources", "locked")}

	got, err := patch.FileSetFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal("./resources/**/*.json")})
	require.NoError(t, err)
	assert.Equal(t, stringSet([]string{"resources/a.json"}), got)
}

func TestFileSetWalksOnlyWhereThePatternCanMatch(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		path    string
		pattern string
		want    []string
	}{
		{
			name:    "depth of the pattern",
			path:    ".",
			pattern: "../*.json",
			want:    []string{"../top.json"},
		},
		{
			name:    "literal directory of the pattern",
			path:    ".",
			pattern: "../shared/**/*.json",
			want:    []string{"../shared/c.json"},
		},
		{
			name:    "literal directory after dot slash",
			path:    "..",
			pattern: "./shared/**/*.json",
			want:    []string{"shared/c.json"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(t, map[string]string{
				"../top.json":        "{}",
				"../shared/c.json":   "{}",
				"../other/deep.json": "{}",
				"main.tf":            "# root",
			})
			fsys := &openFailFS{FS: v.FS, dir: filepath.Join(filepath.Dir(baseDir), "other")}
			v.FS = fsys

			got, err := patch.FileSetFunc(v, baseDir, noRead).
				Call([]cty.Value{cty.StringVal(tc.path), cty.StringVal(tc.pattern)})
			require.NoError(t, err)
			assert.Equal(t, stringSet(tc.want), got)
			assert.False(t, fsys.opened, "fileset opened %s", fsys.dir)
		})
	}
}

func TestFileSetReturnsAnEmptySetForAMissingDirectory(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"main.tf": "# root"})

	got, err := patch.FileSetFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("missing"), cty.StringVal("*.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.SetValEmpty(cty.String), got)
}

// The in-memory filesystem does not list symlinks when reading a directory, so
// the symlink tests run against a temp dir on disk.
//
// TODO: Fix this.
func TestFileSetFollowsSymlinks(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "modules", "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "modules", "a", "main.tf"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shared.tf"), nil, 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir, "modules", "a"), filepath.Join(dir, "linked")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "shared.tf"), filepath.Join(dir, "alias.tf")))

	got, err := patch.FileSetFunc(venvtest.NewWithOSFS(), dir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal("**/*.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.SetVal([]cty.Value{
		cty.StringVal("alias.tf"),
		cty.StringVal("linked/main.tf"),
		cty.StringVal("modules/a/main.tf"),
		cty.StringVal("shared.tf"),
	}), got)
}

func TestFileSetFollowsASymlinkNamedInThePattern(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		pattern string
		want    []string
	}{
		{
			name:    "linked directory after dot slash",
			pattern: "./linked/*.tf",
			want:    []string{"linked/main.tf"},
		},
		{
			name:    "glob under a linked file after dot slash",
			pattern: "./alias.tf/*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := memVenv(t, map[string]string{"modules/a/main.tf": "", "shared.tf": ""})
			require.NoError(t, vfs.Symlink(v.FS, filepath.Join(baseDir, "modules", "a"), filepath.Join(baseDir, "linked")))
			require.NoError(t, vfs.Symlink(v.FS, filepath.Join(baseDir, "shared.tf"), filepath.Join(baseDir, "alias.tf")))

			got, err := patch.FileSetFunc(v, baseDir, noRead).
				Call([]cty.Value{cty.StringVal("."), cty.StringVal(tc.pattern)})
			require.NoError(t, err)
			assert.Equal(t, stringSet(tc.want), got)
		})
	}
}

func TestFileSetResolvesTheParentOfASymlinkedPathAsText(t *testing.T) {
	t.Parallel()

	tcs := []struct {
		name    string
		pattern string
		want    []string
	}{
		{name: "pattern without a directory", pattern: "*.json", want: []string{"lexical.json"}},
		{name: "pattern after dot slash", pattern: "./*.json", want: []string{"lexical.json"}},
		{name: "pattern under a directory", pattern: "sub/*.json", want: []string{"sub/lexical.json"}},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root(baseDir)
			v := memVenv(t, nil).WithFS(venvtest.NewFS(t, root, map[string]string{
				"real/parent/target/main.tf":    "",
				"real/parent/physical.json":     "",
				"real/parent/sub/physical.json": "",
				"live/lexical.json":             "",
				"live/sub/lexical.json":         "",
			}))
			target := filepath.Join(root, "real", "parent", "target")
			require.NoError(t, vfs.Symlink(v.FS, target, filepath.Join(root, "live", "unit")))

			path := filepath.Join(root, "live", "unit") + string(filepath.Separator) + ".."

			got, err := patch.FileSetFunc(v, root, noRead).
				Call([]cty.Value{cty.StringVal(path), cty.StringVal(tc.pattern)})
			require.NoError(t, err)
			assert.Equal(t, stringSet(tc.want), got)
		})
	}
}

func TestFileSetSkipsABrokenSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.Symlink(filepath.Join(dir, "missing.tf"), filepath.Join(dir, "a-broken.tf")))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), nil, 0o644))

	got, err := patch.FileSetFunc(venvtest.NewWithOSFS(), dir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal("*.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.SetVal([]cty.Value{cty.StringVal("main.tf")}), got)
}

func TestFileSetStopsAtASymlinkCycle(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	require.NoError(t, os.Mkdir(filepath.Join(dir, "a"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "main.tf"), nil, 0o644))
	require.NoError(t, os.Symlink(filepath.Join(dir, "a"), filepath.Join(dir, "a", "loop")))

	got, err := patch.FileSetFunc(venvtest.NewWithOSFS(), dir, noRead).
		Call([]cty.Value{cty.StringVal("."), cty.StringVal("**/*.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.SetVal([]cty.Value{
		cty.StringVal("a/loop/main.tf"),
		cty.StringVal("a/main.tf"),
	}), got)
}

func TestFileHashSumsTheVenvContents(t *testing.T) {
	t.Parallel()

	const contents = "hash me\n"

	v := memVenv(t, map[string]string{"data.txt": contents})

	got, err := patch.FileHashFunc(v, baseDir, sha256.New, hex.EncodeToString, noRead).
		Call([]cty.Value{cty.StringVal("data.txt")})
	require.NoError(t, err)

	sum := sha256.Sum256([]byte(contents))
	assert.Equal(t, cty.StringVal(hex.EncodeToString(sum[:])), got)
}

func TestAbsPathResolvesAgainstTheVenvWorkingDir(t *testing.T) {
	t.Parallel()

	wd := venvtest.Root("/work/dir")
	abs := venvtest.Root("/already/absolute.tf")

	v := memVenv(t, nil)
	v.Platform.Getwd = func() (string, error) { return wd, nil }

	fn := patch.AbsPathFunc(v)

	got, err := fn.Call([]cty.Value{cty.StringVal("rel/file.tf")})
	require.NoError(t, err)
	assert.Equal(
		t,
		cty.StringVal(filepath.ToSlash(filepath.Join(wd, "rel/file.tf"))),
		got,
	)

	got, err = fn.Call([]cty.Value{cty.StringVal(abs)})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal(filepath.ToSlash(abs)), got)
}

func TestPathExpandUsesTheVenvHomeDir(t *testing.T) {
	t.Parallel()

	v := memVenv(t, nil)
	v.Platform.UserHomeDir = func() (string, error) { return "/home/tester", nil }

	fn := patch.PathExpandFunc(v)

	got, err := fn.Call([]cty.Value{cty.StringVal("~/configs/main.tf")})
	require.NoError(t, err)
	assert.Equal(
		t,
		cty.StringVal(filepath.Join("/home/tester", "configs/main.tf")),
		got,
	)

	got, err = fn.Call([]cty.Value{cty.StringVal("relative/path.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("relative/path.tf"), got)
}

func TestPathExpandAcceptsABackslashAfterTheTilde(t *testing.T) {
	t.Parallel()

	v := memVenv(t, nil)
	v.Platform.UserHomeDir = func() (string, error) { return "/home/tester", nil }

	got, err := patch.PathExpandFunc(v).
		Call([]cty.Value{cty.StringVal(`~\configs\main.tf`)})
	require.NoError(t, err)
	assert.Equal(
		t,
		cty.StringVal(filepath.Join("/home/tester", `\configs\main.tf`)),
		got,
	)
}

func TestPathExpandRejectsAUserSpecificHomeDir(t *testing.T) {
	t.Parallel()

	v := memVenv(t, nil)
	v.Platform.UserHomeDir = func() (string, error) { return "/home/tester", nil }

	_, err := patch.PathExpandFunc(v).
		Call([]cty.Value{cty.StringVal("~alice/main.tf")})
	require.ErrorIs(t, err, patch.ErrUserSpecificHomeDir)
}

func TestFileExistsRejectsAUserSpecificHomeDir(t *testing.T) {
	t.Parallel()

	v := memVenv(t, map[string]string{"~alice/main.tf": "# shadow"})

	_, err := patch.FileExistsFunc(v, baseDir, noRead).
		Call([]cty.Value{cty.StringVal("~alice/main.tf")})
	require.ErrorIs(t, err, patch.ErrUserSpecificHomeDir)
}

func TestBase64DecodeReturnsTheDecodedString(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	encoded := base64.StdEncoding.EncodeToString([]byte("plain text"))

	got, err := patch.Base64DecodeFunc(l).Call([]cty.Value{cty.StringVal(encoded)})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("plain text"), got)
}

func TestBase64DecodeRejectsContentsThatAreNotUTF8(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	encoded := base64.StdEncoding.EncodeToString([]byte("\xff\xfe binary"))

	_, err := patch.Base64DecodeFunc(l).Call([]cty.Value{cty.StringVal(encoded)})
	require.ErrorIs(t, err, patch.ErrBase64NotUTF8)
}

func TestTemplateFileRendersFromTheVenv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := memVenv(t, map[string]string{"greeting.tmpl": "Hello, ${name}!"})

	fn := patch.TemplateFileFunc(
		v,
		l,
		baseDir,
		func() map[string]function.Function { return nil },
		noRead,
	)

	got, err := fn.Call([]cty.Value{
		cty.StringVal("greeting.tmpl"),
		cty.ObjectVal(map[string]cty.Value{"name": cty.StringVal("world")}),
	})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("Hello, world!"), got)
}

func TestTemplateFileStopsAtTheRecursionCap(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := memVenv(t, map[string]string{
		"loop.tmpl": `${templatefile("loop.tmpl", {})}`,
	})
	v.Env[recursionDepthEnv] = "4"

	var table map[string]function.Function

	fn := patch.TemplateFileFunc(
		v,
		l,
		baseDir,
		func() map[string]function.Function { return table },
		noRead,
	)
	table = map[string]function.Function{"templatefile": fn}

	_, err := fn.Call([]cty.Value{
		cty.StringVal("loop.tmpl"),
		cty.EmptyObjectVal,
	})
	require.Error(t, err)
}

// noRead is the read observer for tests that do not inspect what was read.
func noRead(string) {}

func stringSet(paths []string) cty.Value {
	if len(paths) == 0 {
		return cty.SetValEmpty(cty.String)
	}

	vals := make([]cty.Value, 0, len(paths))
	for _, path := range paths {
		vals = append(vals, cty.StringVal(path))
	}

	return cty.SetVal(vals)
}

var errOpenInjected = errors.New("injected open failure")

type openFailFS struct {
	vfs.FS
	dir    string
	opened bool
}

func (fsys *openFailFS) Open(name string) (vfs.File, error) {
	if filepath.Clean(name) != fsys.dir {
		return fsys.FS.Open(name)
	}

	fsys.opened = true

	return nil, errOpenInjected
}

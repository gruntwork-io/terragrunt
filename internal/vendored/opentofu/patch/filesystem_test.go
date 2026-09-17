package patch_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

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

	return venvtest.New().WithFS(venvtest.NewFS(t, baseDir, files))
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

	v := venvtest.New().WithFS(fsys)
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

	v := memVenv(t, nil)
	v.Platform.Getwd = func() (string, error) { return "/work/dir", nil }

	fn := patch.AbsPathFunc(v)

	got, err := fn.Call([]cty.Value{cty.StringVal("rel/file.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("/work/dir/rel/file.tf"), got)

	got, err = fn.Call([]cty.Value{cty.StringVal("/already/absolute.tf")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("/already/absolute.tf"), got)
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

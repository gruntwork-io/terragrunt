package util

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"testing"

	"fmt"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathRelativeTo(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		basePath string
		expected string
	}{
		{"", "", "."},
		{helpers.RootFolder, helpers.RootFolder, "."},
		{helpers.RootFolder, helpers.RootFolder + "child", ".."},
		{helpers.RootFolder, helpers.RootFolder + "child/sub-child/sub-sub-child", "../../.."},
		{helpers.RootFolder + "other-child", helpers.RootFolder + "child", "../other-child"},
		{helpers.RootFolder + "other-child/sub-child", helpers.RootFolder + "child/sub-child", "../../other-child/sub-child"},
		{helpers.RootFolder + "root", helpers.RootFolder + "other-root", "../root"},
		{helpers.RootFolder + "root", helpers.RootFolder + "other-root/sub-child/sub-sub-child", "../../../root"},
	}

	for _, testCase := range testCases {
		actual, err := GetPathRelativeTo(testCase.path, testCase.basePath)
		assert.Nil(t, err, "Unexpected error for path %s and basePath %s: %v", testCase.path, testCase.basePath, err)
		assert.Equal(t, testCase.expected, actual, "For path %s and basePath %s", testCase.path, testCase.basePath)
	}
}

func TestCanonicalPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		basePath string
		expected string
	}{
		{"", helpers.RootFolder + "foo", helpers.RootFolder + "foo"},
		{".", helpers.RootFolder + "foo", helpers.RootFolder + "foo"},
		{"bar", helpers.RootFolder + "foo", helpers.RootFolder + "foo/bar"},
		{"bar/baz/blah", helpers.RootFolder + "foo", helpers.RootFolder + "foo/bar/baz/blah"},
		{"bar/../blah", helpers.RootFolder + "foo", helpers.RootFolder + "foo/blah"},
		{"bar/../..", helpers.RootFolder + "foo", helpers.RootFolder},
		{"bar/.././../baz", helpers.RootFolder + "foo", helpers.RootFolder + "baz"},
		{"bar", helpers.RootFolder + "foo/../baz", helpers.RootFolder + "baz/bar"},
		{"a/b/../c/d/..", helpers.RootFolder + "foo/../baz/.", helpers.RootFolder + "baz/a/c"},
		{helpers.RootFolder + "other", helpers.RootFolder + "foo", helpers.RootFolder + "other"},
		{helpers.RootFolder + "other/bar/blah", helpers.RootFolder + "foo", helpers.RootFolder + "other/bar/blah"},
		{helpers.RootFolder + "other/../blah", helpers.RootFolder + "foo", helpers.RootFolder + "blah"},
	}

	for _, testCase := range testCases {
		actual, err := CanonicalPath(testCase.path, testCase.basePath)
		assert.Nil(t, err, "Unexpected error for path %s and basePath %s: %v", testCase.path, testCase.basePath, err)
		assert.Equal(t, testCase.expected, actual, "For path %s and basePath %s", testCase.path, testCase.basePath)
	}
}

func TestGlobCanonicalPath(t *testing.T) {
	t.Parallel()

	basePath := "testdata/fixture-glob-canonical"

	expectedHelper := func(path string) string {
		basePath, err := filepath.Abs(basePath)
		assert.NoError(t, err)
		return filepath.ToSlash(filepath.Join(basePath, path))
	}

	testCases := []struct {
		paths    []string
		expected []string
	}{
		{[]string{"module-a", "module-b/module-b-child/.."}, []string{expectedHelper("module-a"), expectedHelper("module-b")}},
		{[]string{"*-a", "*-b"}, []string{expectedHelper("module-a"), expectedHelper("module-b")}},
		{[]string{"module-*"}, []string{expectedHelper("module-a"), expectedHelper("module-b")}},
		{[]string{"module-*/*.hcl"}, []string{expectedHelper("module-a/terragrunt.hcl"), expectedHelper("module-b/terragrunt.hcl")}},
		{[]string{"module-*/**/*.hcl"}, []string{expectedHelper("module-a/terragrunt.hcl"), expectedHelper("module-b/terragrunt.hcl"), expectedHelper("module-b/module-b-child/terragrunt.hcl")}},
	}

	for _, testCase := range testCases {
		actual, err := GlobCanonicalPath(basePath, testCase.paths...)

		sort.Slice(actual, func(i, j int) bool {
			return actual[i] < actual[j]
		})

		sort.Slice(testCase.expected, func(i, j int) bool {
			return testCase.expected[i] < testCase.expected[j]
		})

		assert.Nil(t, err, "Unexpected error for paths %s and basePath %s: %v", testCase.paths, basePath, err)
		assert.Equal(t, testCase.expected, actual, "For path %s and basePath %s", testCase.paths, basePath)
	}
}

func TestPathContainsHiddenFileOrFolder(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		expected bool
	}{
		{"", false},
		{".", false},
		{".foo", true},
		{".foo/", true},
		{"foo/bar", false},
		{"/foo/bar", false},
		{".foo/bar", true},
		{"foo/.bar", true},
		{"/foo/.bar", true},
		{"/foo/./bar", false},
		{"/foo/../bar", false},
		{"/foo/.././bar", false},
		{"/foo/.././.bar", true},
		{"/foo/.././.bar/", true},
	}

	for _, testCase := range testCases {
		path := filepath.FromSlash(testCase.path)
		actual := TerragruntExcludes(path)
		assert.Equal(t, testCase.expected, actual, "For path %s", path)
	}
}

func TestJoinTerraformModulePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		modulesFolder string
		path          string
		expected      string
	}{
		{"foo", "bar", "foo//bar"},
		{"foo/", "bar", "foo//bar"},
		{"foo", "/bar", "foo//bar"},
		{"foo/", "/bar", "foo//bar"},
		{"foo//", "/bar", "foo//bar"},
		{"foo//", "//bar", "foo//bar"},
		{"/foo/bar/baz", "/a/b/c", "/foo/bar/baz//a/b/c"},
		{"/foo/bar/baz/", "//a/b/c", "/foo/bar/baz//a/b/c"},
		{"/foo?ref=feature/1", "bar", "/foo//bar?ref=feature/1"},
		{"/foo?ref=feature/1", "/bar", "/foo//bar?ref=feature/1"},
		{"/foo//?ref=feature/1", "/bar", "/foo//bar?ref=feature/1"},
		{"/foo//?ref=feature/1", "//bar", "/foo//bar?ref=feature/1"},
		{"/foo/bar/baz?ref=feature/1", "/a/b/c", "/foo/bar/baz//a/b/c?ref=feature/1"},
		{"/foo/bar/baz/?ref=feature/1", "//a/b/c", "/foo/bar/baz//a/b/c?ref=feature/1"},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("%s-%s", testCase.modulesFolder, testCase.path), func(t *testing.T) {
			actual := JoinTerraformModulePath(testCase.modulesFolder, testCase.path)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestFileManifest(t *testing.T) {
	t.Parallel()

	var testfiles []string

	// create temp dir
	dir, err := os.MkdirTemp("", ".terragrunt-test-dir")
	require.NoError(t, err)
	for _, file := range []string{"file1", "file2"} {
		// create temp files in the dir
		f, err := os.CreateTemp(dir, file)
		assert.NoError(t, err, f.Close())
		testfiles = append(testfiles, f.Name())
	}
	// will later test if the file already doesn't exist
	testfiles = append(testfiles, path.Join(dir, "ephemeral-file-that-doesnt-exist.txt"))

	// create a manifest
	manifest := newFileManifest(dir, ".terragrunt-test-manifest")
	require.Nil(t, manifest.Create())
	// check the file manifest has been created
	require.FileExists(t, filepath.Join(manifest.ManifestFolder, manifest.ManifestFile))
	for _, file := range testfiles {
		assert.NoError(t, manifest.AddFile(file))
	}
	// check for a non-existent directory as well
	assert.NoError(t, manifest.AddDirectory(path.Join(dir, "ephemeral-directory-that-doesnt-exist")))

	require.NoError(t, manifest.Clean())
	// test if the files have been deleted
	for _, file := range testfiles {
		assert.Equal(t, FileExists(file), false)
	}

}

func TestSplitPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		expected []string
	}{
		{"foo/bar/.tf/tg.hcl", []string{"foo", "bar", ".tf", "tg.hcl"}},
		{"/foo/bar/.tf/tg.hcl", []string{"", "foo", "bar", ".tf", "tg.hcl"}},
		{"../foo/bar/.tf/tg.hcl", []string{"..", "foo", "bar", ".tf", "tg.hcl"}},
		{"foo//////bar/.tf/tg.hcl", []string{"foo", "bar", ".tf", "tg.hcl"}},
	}

	for _, testCase := range testCases {
		actual := SplitPath(testCase.path)
		assert.Equal(t, testCase.expected, actual, "For path %s", testCase.path)
	}
}

func TestContainsPath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		subpath  string
		expected bool
	}{
		{"", "", true},
		{"/", "/", true},
		{"foo/bar/.tf/tg.hcl", "foo/bar", true},
		{"/foo/bar/.tf/tg.hcl", "foo/bar", true},
		{"foo/bar/.tf/tg.hcl", "bar", true},
		{"foo/bar/.tf/tg.hcl", ".tf/tg.hcl", true},
		{"foo/bar/.tf/tg.hcl", "tg.hcl", true},

		{"foo/bar/.tf/tg.hcl", "/bar", false},
		{"/foo/bar/.tf/tg.hcl", "/bar", false},
		{"foo/bar", "foo/bar/gee", false},
		{"foo/bar/.tf/tg.hcl", "foo/barf", false},
		{"foo/bar/.tf/tg.hcl", "foo/ba", false},
	}

	for _, testCase := range testCases {
		actual := ContainsPath(testCase.path, testCase.subpath)
		assert.Equal(t, testCase.expected, actual, "For path %s and subpath %s", testCase.path, testCase.subpath)
	}
}

func TestHasPathPrefix(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path     string
		prefix   string
		expected bool
	}{
		{"", "", true},
		{"/", "/", true},
		{"foo/bar/.tf/tg.hcl", "foo", true},
		{"/foo/bar/.tf/tg.hcl", "/foo", true},
		{"foo/bar/.tf/tg.hcl", "foo/bar", true},
		{"/foo/bar/.tf/tg.hcl", "/foo/bar", true},

		{"/", "", false},
		{"foo", "foo/bar/.tf/tg.hcl", false},
		{"/foo/bar/.tf/tg.hcl", "foo", false},
		{"/foo/bar/.tf/tg.hcl", "bar/.tf", false},
		{"/foo/bar/.tf/tg.hcl", "/foo/barf", false},
		{"/foo/bar/.tf/tg.hcl", "/foo/ba", false},
	}

	for _, testCase := range testCases {
		actual := HasPathPrefix(testCase.path, testCase.prefix)
		assert.Equal(t, testCase.expected, actual, "For path %s and prefix %s", testCase.path, testCase.prefix)
	}
}

func TestIncludeInCopy(t *testing.T) {
	includeInCopy := []string{"_module/.region2", "**/app2", "**/.include-me-too"}

	testCases := []struct {
		path         string
		copyExpected bool
	}{
		{"/app/terragrunt.hcl", true},
		{"/_module/main.tf", true},
		{"/_module/.region1/info.txt", false},
		{"/_module/.region3/project3-1/f1-2-levels.txt", false},
		{"/_module/.region3/project3-1/app1/.include-me-too/file.txt", true},
		{"/_module/.region3/project3-2/.f0/f0-3-levels.txt", false},
		{"/_module/.region2/.project2-1/app2/f2-dot-f2.txt", true},
		{"/_module/.region2/.project2-1/readme.txt", true},
		{"/_module/.region2/project2-2/f2-dot-f0.txt", true},
	}

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source")
	destination := filepath.Join(tempDir, "destination")

	fileContent := []byte("source file")
	for _, testCase := range testCases {
		path := filepath.Join(source, testCase.path)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
		require.NoError(t, os.WriteFile(path, fileContent, 0644))
	}

	require.NoError(t, CopyFolderContents(source, destination, ".terragrunt-test", includeInCopy))

	for _, testCase := range testCases {
		_, err := os.Stat(filepath.Join(destination, testCase.path))
		assert.True(t,
			testCase.copyExpected && err == nil ||
				!testCase.copyExpected && errors.Is(err, os.ErrNotExist),
			"Unexpected copy result for file '%s' (should be copied: '%t') - got error: %s", testCase.path, testCase.copyExpected, err)
	}
}

func TestEmptyDir(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		path        string
		expectEmpty bool
	}{
		{t.TempDir(), true},
		{os.TempDir(), false},
	}
	for _, testCase := range testCases {
		emptyValue, err := IsDirectoryEmpty(testCase.path)
		assert.NoError(t, err)
		assert.Equal(t, testCase.expectEmpty, emptyValue, "For path %s", testCase.path)
	}

}

package util_test

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"fmt"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathRelativeTo(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.GetPathRelativeTo(tt.path, tt.basePath)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual, "For path %s and basePath %s", tt.path, tt.basePath)
		})
	}
}

func TestCanonicalPath(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.CanonicalPath(tt.path, tt.basePath)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual, "For path %s and basePath %s", tt.path, tt.basePath)
		})
	}
}

func TestGlobCanonicalPath(t *testing.T) {
	t.Parallel()

	basePath := "testdata/fixture-glob-canonical"

	expectedHelper := func(path string) string {
		basePath, err := filepath.Abs(basePath)
		require.NoError(t, err)
		return filepath.ToSlash(filepath.Join(basePath, path))
	}

	tc := []struct {
		paths    []string
		expected []string
	}{
		{[]string{"module-a", "module-b/module-b-child/.."}, []string{expectedHelper("module-a"), expectedHelper("module-b")}},
		{[]string{"*-a", "*-b"}, []string{expectedHelper("module-a"), expectedHelper("module-b")}},
		{[]string{"module-*"}, []string{expectedHelper("module-a"), expectedHelper("module-b")}},
		{[]string{"module-*/*.hcl"}, []string{expectedHelper("module-a/terragrunt.hcl"), expectedHelper("module-b/terragrunt.hcl")}},
		{[]string{"module-*/**/*.hcl"}, []string{expectedHelper("module-a/terragrunt.hcl"), expectedHelper("module-b/terragrunt.hcl"), expectedHelper("module-b/module-b-child/terragrunt.hcl")}},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.GlobCanonicalPath(basePath, tt.paths...)

			sort.Slice(actual, func(i, j int) bool {
				return actual[i] < actual[j]
			})

			sort.Slice(tt.expected, func(i, j int) bool {
				return tt.expected[i] < tt.expected[j]
			})

			require.NoError(t, err)
			assert.Equal(t, tt.expected, actual, "For path %s and basePath %s", tt.paths, basePath)
		})
	}
}

func TestPathContainsHiddenFileOrFolder(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for _, tt := range tc {
		tt := tt

		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()

			path := filepath.FromSlash(tt.path)
			actual := util.TerragruntExcludes(path)
			assert.Equal(t, tt.expected, actual, "For path %s", path)
		})
	}
}

func TestJoinTerraformModulePath(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for _, tt := range tc {
		tt := tt

		t.Run(fmt.Sprintf("%s-%s", tt.modulesFolder, tt.path), func(t *testing.T) {
			t.Parallel()

			actual := util.JoinTerraformModulePath(tt.modulesFolder, tt.path)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestFileManifest(t *testing.T) {
	t.Parallel()

	files := []string{"file1", "file2"}
	var testfiles = make([]string, 0, len(files))

	// create temp dir
	dir, err := os.MkdirTemp("", ".terragrunt-test-dir")
	require.NoError(t, err)
	for _, file := range files {
		// create temp files in the dir
		f, err := os.CreateTemp(dir, file)
		require.NoError(t, err)
		testfiles = append(testfiles, f.Name())
	}
	// will later test if the file already doesn't exist
	testfiles = append(testfiles, path.Join(dir, "ephemeral-file-that-doesnt-exist.txt"))

	// create a manifest
	manifest := util.NewFileManifest(dir, ".terragrunt-test-manifest")
	require.NoError(t, manifest.Create())
	// check the file manifest has been created
	assert.FileExists(t, filepath.Join(manifest.ManifestFolder, manifest.ManifestFile))
	for _, file := range testfiles {
		require.NoError(t, manifest.AddFile(file))
	}
	// check for a non-existent directory as well
	assert.NoError(t, manifest.AddDirectory(path.Join(dir, "ephemeral-directory-that-doesnt-exist")))

	assert.NoError(t, manifest.Clean())
	// test if the files have been deleted
	for _, file := range testfiles {
		assert.False(t, util.FileExists(file))
	}

}

func TestSplitPath(t *testing.T) {
	t.Parallel()

	tc := []struct {
		path     string
		expected []string
	}{
		{"foo/bar/.tf/tg.hcl", []string{"foo", "bar", ".tf", "tg.hcl"}},
		{"/foo/bar/.tf/tg.hcl", []string{"", "foo", "bar", ".tf", "tg.hcl"}},
		{"../foo/bar/.tf/tg.hcl", []string{"..", "foo", "bar", ".tf", "tg.hcl"}},
		{"foo//////bar/.tf/tg.hcl", []string{"foo", "bar", ".tf", "tg.hcl"}},
	}

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.SplitPath(tt.path)
			assert.Equal(t, tt.expected, actual, "For path %s", tt.path)
		})
	}
}

func TestContainsPath(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			actual := util.ContainsPath(tt.path, tt.subpath)
			assert.Equal(t, tt.expected, actual, "For path %s and subpath %s", tt.path, tt.subpath)
		})
	}
}

func TestHasPathPrefix(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.HasPathPrefix(tt.path, tt.prefix)
			assert.Equal(t, tt.expected, actual, "For path %s and prefix %s", tt.path, tt.prefix)
		})
	}
}

func TestIncludeInCopy(t *testing.T) {
	t.Parallel()

	includeInCopy := []string{"_module/.region2", "**/app2", "**/.include-me-too"}

	tc := []struct {
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
	for _, tt := range tc {
		path := filepath.Join(source, tt.path)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
		assert.NoError(t, os.WriteFile(path, fileContent, 0644))
	}

	require.NoError(t, util.CopyFolderContents(source, destination, ".terragrunt-test", includeInCopy))

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			_, err := os.Stat(filepath.Join(destination, tt.path))
			assert.True(t,
				tt.copyExpected && err == nil ||
					!tt.copyExpected && errors.Is(err, os.ErrNotExist),
				"Unexpected copy result for file '%s' (should be copied: '%t') - got error: %s", tt.path, tt.copyExpected, err)
		})
	}
}

func TestEmptyDir(t *testing.T) {
	t.Parallel()
	tc := []struct {
		path        string
		expectEmpty bool
	}{
		{t.TempDir(), true},
		{os.TempDir(), false},
	}
	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			emptyValue, err := util.IsDirectoryEmpty(tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.expectEmpty, emptyValue, "For path %s", tt.path)
		})
	}
}

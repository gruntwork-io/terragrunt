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

	"github.com/gruntwork-io/terragrunt/pkg/log"
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
		{[]string{"module-*/*.hcl"}, []string{expectedHelper("module-a/terragrunt.hcl"), expectedHelper("module-b/root.hcl")}},
		{[]string{"module-*/**/*.hcl"}, []string{expectedHelper("module-a/terragrunt.hcl"), expectedHelper("module-b/root.hcl"), expectedHelper("module-b/module-b-child/terragrunt.hcl")}},
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
	manifest := util.NewFileManifest(log.New(), dir, ".terragrunt-test-manifest")
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

	require.NoError(t, util.CopyFolderContents(log.New(), source, destination, ".terragrunt-test", includeInCopy, nil))

	for i, tt := range tc {

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

func TestExcludeFromCopy(t *testing.T) {
	t.Parallel()

	excludeFromCopy := []string{"module/region2", "**/exclude-me-here", "**/app1"}

	testCases := []struct {
		path         string
		copyExpected bool
	}{
		{"/app/terragrunt.hcl", true},
		{"/module/main.tf", true},
		{"/module/region1/info.txt", true},
		{"/module/region1/project2-1/app1/f2-dot-f2.txt", false},
		{"/module/region3/project3-1/f1-2-levels.txt", true},
		{"/module/region3/project3-1/app1/exclude-me-here/file.txt", false},
		{"/module/region3/project3-2/f0/f0-3-levels.txt", true},
		{"/module/region2/project2-1/app2/f2-dot-f2.txt", false},
		{"/module/region2/project2-1/readme.txt", false},
		{"/module/region2/project2-2/f2-dot-f0.txt", false},
	}

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source")
	destination := filepath.Join(tempDir, "destination")

	fileContent := []byte("source file")
	for _, tt := range testCases {
		path := filepath.Join(source, tt.path)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
		assert.NoError(t, os.WriteFile(path, fileContent, 0644))
	}

	require.NoError(t, util.CopyFolderContents(log.New(), source, destination, ".terragrunt-test", nil, excludeFromCopy))

	for i, testCase := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			_, err := os.Stat(filepath.Join(destination, testCase.path))
			assert.True(t,
				testCase.copyExpected && err == nil ||
					!testCase.copyExpected && errors.Is(err, os.ErrNotExist),
				"Unexpected copy result for file '%s' (should be copied: '%t') - got error: %s", testCase.path, testCase.copyExpected, err)
		})
	}
}

func TestExcludeIncludeBehaviourPriority(t *testing.T) {
	t.Parallel()

	includeInCopy := []string{"_module/.region2", "_module/.region3"}
	excludeFromCopy := []string{"**/.project2-2", "_module/.region3"}

	testCases := []struct {
		path         string
		copyExpected bool
	}{
		{"/_module/.region2/.project2-1/app2/f2-dot-f2.txt", true},
		{"/_module/.region2/.project2-1/readme.txt", true},
		{"/_module/.region2/.project2-2/f2-dot-f0.txt", false},
		{"/_module/.region3/.project2-1/readme.txt", false},
	}

	tempDir := t.TempDir()
	source := filepath.Join(tempDir, "source")
	destination := filepath.Join(tempDir, "destination")

	fileContent := []byte("source file")
	for _, tt := range testCases {
		path := filepath.Join(source, tt.path)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
		assert.NoError(t, os.WriteFile(path, fileContent, 0644))
	}

	require.NoError(t, util.CopyFolderContents(log.New(), source, destination, ".terragrunt-test", includeInCopy, excludeFromCopy))

	for i, tt := range testCases {
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

//nolint:funlen
func TestWalkWithSimpleSymlinks(t *testing.T) {
	t.Parallel()
	// Create temporary test directory structure
	tempDir := t.TempDir()
	tempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Create directories
	dirs := []string{"a", "d"}
	for _, dir := range dirs {
		require.NoError(t, os.Mkdir(filepath.Join(tempDir, dir), 0755))
	}

	// Create test files
	testFile := filepath.Join(tempDir, "a", "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	// Create symlinks
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "b")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "c")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "d", "a")))

	var paths []string
	err = util.WalkWithSymlinks(tempDir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, relPath)

		return nil
	})
	require.NoError(t, err)

	// Sort paths for reliable comparison
	sort.Strings(paths)

	// Expected paths should include original and symlinked locations
	expectedPaths := []string{
		".",
		"a",
		"a/test.txt",
		"b",
		"b/test.txt",
		"c",
		"c/test.txt",
		"d",
		"d/a",
		"d/a/test.txt",
	}
	sort.Strings(expectedPaths)

	if len(paths) != len(expectedPaths) {
		t.Errorf("Got %d paths, expected %d", len(paths), len(expectedPaths))
	}

	for expectedPath := range expectedPaths {
		if expectedPath >= len(paths) {
			t.Errorf("Missing expected path: %s", expectedPaths[expectedPath])

			continue
		}
		if paths[expectedPath] != expectedPaths[expectedPath] {
			t.Errorf("Path mismatch at index %d:\ngot:  %s\nwant: %s", expectedPath, paths[expectedPath], expectedPaths[expectedPath])
		}
	}
}

//nolint:funlen
func TestWalkWithCircularSymlinks(t *testing.T) {
	t.Parallel()
	// Create temporary test directory structure
	tempDir := t.TempDir()
	tempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Create directories
	dirs := []string{"a", "b", "c", "d"}
	for _, dir := range dirs {
		require.NoError(t, os.Mkdir(filepath.Join(tempDir, dir), 0755))
	}

	// Create test files
	testFile := filepath.Join(tempDir, "a", "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))

	// Create symlinks
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "b", "link-to-a")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "c", "another-link-to-a")))

	// Create circular symlink
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "d"), filepath.Join(tempDir, "a", "link-to-d")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "d", "link-to-a")))

	var paths []string
	err = util.WalkWithSymlinks(tempDir, func(path string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, relPath)

		return nil
	})
	require.NoError(t, err)

	// Sort paths for reliable comparison
	sort.Strings(paths)

	// Expected paths should include original and symlinked locations
	expectedPaths := []string{
		".",
		"a",
		"a/link-to-d",
		"a/link-to-d/link-to-a",
		"a/link-to-d/link-to-a/link-to-d",
		"a/link-to-d/link-to-a/test.txt",
		"a/test.txt",
		"b",
		"b/link-to-a",
		"b/link-to-a/link-to-d",
		"b/link-to-a/test.txt",
		"c",
		"c/another-link-to-a",
		"c/another-link-to-a/link-to-d",
		"c/another-link-to-a/test.txt",
		"d",
		"d/link-to-a",
	}
	sort.Strings(expectedPaths)

	if len(paths) != len(expectedPaths) {
		t.Errorf("Got %d paths, expected %d", len(paths), len(expectedPaths))
	}

	for expectedPath := range expectedPaths {
		if expectedPath >= len(paths) {
			t.Errorf("Missing expected path: %s", expectedPaths[expectedPath])

			continue
		}
		if paths[expectedPath] != expectedPaths[expectedPath] {
			t.Errorf("Path mismatch at index %d:\ngot:  %s\nwant: %s", expectedPath, paths[expectedPath], expectedPaths[expectedPath])
		}
	}
}

func TestWalkWithSymlinksErrors(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Test with non-existent directory
	require.Error(t, util.WalkWithSymlinks(filepath.Join(tempDir, "nonexistent"), func(_ string, _ os.FileInfo, err error) error {
		return err
	}))

	// Test with broken symlink
	brokenLink := filepath.Join(tempDir, "broken")
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "nonexistent"), brokenLink))

	require.Error(t, util.WalkWithSymlinks(tempDir, func(_ string, _ os.FileInfo, err error) error {
		return err
	}))
}

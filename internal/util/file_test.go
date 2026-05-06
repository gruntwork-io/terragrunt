package util_test

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testManifestName = ".terragrunt-test-manifest"

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

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual, err := util.CanonicalPath(tc.path, tc.basePath)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, actual, "For path %s and basePath %s", tc.path, tc.basePath)
		})
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

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			path := filepath.FromSlash(tc.path)
			actual := util.TerragruntExcludes(path)
			assert.Equal(t, tc.expected, actual, "For path %s", path)
		})
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

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s-%s", tc.modulesFolder, tc.path), func(t *testing.T) {
			t.Parallel()

			actual := util.JoinTerraformModulePath(tc.modulesFolder, tc.path)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestFileManifest(t *testing.T) {
	t.Parallel()

	files := []string{"file1", "file2"}

	testfiles := make([]string, 0, len(files))

	// create temp dir
	dir := helpers.TmpDirWOSymlinks(t)

	for _, file := range files {
		// create temp files in the dir
		f, err := os.CreateTemp(dir, file)
		require.NoError(t, err)
		// Close the file handle immediately after creation
		require.NoError(t, f.Close())
		testfiles = append(testfiles, f.Name())
	}
	// will later test if the file already doesn't exist
	testfiles = append(testfiles, path.Join(dir, "ephemeral-file-that-doesnt-exist.txt"))

	// create a manifest
	l := logger.CreateLogger()
	manifest := util.NewFileManifest(dir, testManifestName)
	require.NoError(t, manifest.Create())
	// check the file manifest has been created
	assert.FileExists(t, filepath.Join(manifest.ManifestFolder, manifest.ManifestFile))

	for _, file := range testfiles {
		require.NoError(t, manifest.AddFile(file))
	}
	// check for a non-existent directory as well
	assert.NoError(t, manifest.AddDirectory(path.Join(dir, "ephemeral-directory-that-doesnt-exist")))

	// Close the manifest file handle before cleaning
	require.NoError(t, manifest.Close())

	assert.NoError(t, manifest.Clean(l))
	// test if the files have been deleted
	for _, file := range testfiles {
		assert.False(t, util.FileExists(file))
	}
}

// TestFileManifestCleanRejectsOutOfRootEntry pins that manifests cannot drive out-of-root cleanup.
func TestFileManifestCleanRejectsOutOfRootEntry(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	outsideDir := helpers.TmpDirWOSymlinks(t)
	sentinel := filepath.Join(outsideDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("must survive"), 0o600))

	root := helpers.TmpDirWOSymlinks(t)
	manifestName := testManifestName
	manifestPath := filepath.Join(root, manifestName)

	writeManifest(t, manifestPath, sentinel)

	manifest := util.NewFileManifest(root, manifestName)
	require.NoError(t, manifest.Clean(l))

	assert.FileExists(t, sentinel, "out-of-root manifest entry must be ignored")
}

func TestFileManifestCleanRemovesInRootEntry(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	root := helpers.TmpDirWOSymlinks(t)
	staleFile := filepath.Join(root, "stale.tf")
	require.NoError(t, os.WriteFile(staleFile, []byte("stale"), 0o600))

	manifestName := testManifestName
	writeManifest(t, filepath.Join(root, manifestName), staleFile)

	manifest := util.NewFileManifest(root, manifestName)
	require.NoError(t, manifest.Clean(l))

	assert.NoFileExists(t, staleFile, "in-root manifest entry must still be cleaned")
}

func TestFileManifestCleanRemovesRelativeInRootEntry(t *testing.T) { //nolint:paralleltest // depends on stable process CWD
	l := logger.CreateLogger()

	root := helpers.TmpDirWOSymlinks(t)
	staleFile := filepath.Join(root, "stale.tf")
	require.NoError(t, os.WriteFile(staleFile, []byte("stale"), 0o600))

	cwd, err := os.Getwd()
	require.NoError(t, err)

	rootRel, err := filepath.Rel(cwd, root)
	require.NoError(t, err)

	staleFileRel, err := filepath.Rel(cwd, staleFile)
	require.NoError(t, err)

	manifestName := testManifestName
	writeManifest(t, filepath.Join(rootRel, manifestName), staleFileRel)

	manifest := util.NewFileManifest(rootRel, manifestName)
	require.NoError(t, manifest.Clean(l))

	assert.NoFileExists(t, staleFile, "relative in-root manifest entry must still be cleaned")
}

// TestFileManifestCleanRejectsCreatedOutOfRootEntries pins that Terragrunt-created manifests cannot remove outside ManifestFolder.
func TestFileManifestCleanRejectsCreatedOutOfRootEntries(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	outsideDir := helpers.TmpDirWOSymlinks(t)
	sentinel := filepath.Join(outsideDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("must survive"), 0o600))

	root := helpers.TmpDirWOSymlinks(t)
	manifest := util.NewFileManifest(root, testManifestName)
	require.NoError(t, manifest.Create())
	require.NoError(t, manifest.AddFile(sentinel))
	require.NoError(t, manifest.Close())

	require.NoError(t, manifest.Clean(l))

	assert.FileExists(t, sentinel, "out-of-root entry must not be removed from a Terragrunt-created manifest")
}

func TestFileManifestCleanRejectsSymlinkEscapes(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	outsideDir := helpers.TmpDirWOSymlinks(t)
	sentinel := filepath.Join(outsideDir, "sentinel.txt")
	require.NoError(t, os.WriteFile(sentinel, []byte("must survive"), 0o600))

	root := helpers.TmpDirWOSymlinks(t)
	if err := os.Symlink(outsideDir, filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks are not available: %v", err)
	}

	manifest := util.NewFileManifest(root, testManifestName)
	require.NoError(t, manifest.Create())
	require.NoError(t, manifest.AddFile(filepath.Join(root, "link", "sentinel.txt")))
	require.NoError(t, manifest.Close())

	require.NoError(t, manifest.Clean(l))

	assert.FileExists(t, sentinel, "manifest cleanup must not follow symlink parents outside the root")
}

func TestFileManifestCleanRemovesManifestNamedDirectory(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	root := helpers.TmpDirWOSymlinks(t)
	manifestName := testManifestName
	manifestDir := filepath.Join(root, manifestName)
	require.NoError(t, os.MkdirAll(manifestDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(manifestDir, "trapped.tf"), []byte("trap"), 0o600))

	manifest := util.NewFileManifest(root, manifestName)
	require.NoError(t, manifest.Clean(l))
	require.NoDirExists(t, manifestDir)
}

func TestFileManifestCleanRemovesNestedManifestNamedDirectory(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	root := helpers.TmpDirWOSymlinks(t)
	manifestName := testManifestName
	nestedDir := filepath.Join(root, "sub")
	nestedManifestDir := filepath.Join(nestedDir, manifestName)
	require.NoError(t, os.MkdirAll(nestedManifestDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(nestedManifestDir, "trapped.tf"), []byte("trap"), 0o600))

	manifest := util.NewFileManifest(root, manifestName)
	require.NoError(t, manifest.Create())
	require.NoError(t, manifest.AddDirectory(nestedDir))
	require.NoError(t, manifest.Close())

	require.NoError(t, manifest.Clean(l))

	require.NoDirExists(t, nestedManifestDir)
}

func TestFileManifestCleanRejectsSelfReferencingDirectoryCycle(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	root := helpers.TmpDirWOSymlinks(t)
	manifestPath := filepath.Join(root, testManifestName)
	nestedDir := filepath.Join(root, "sub")
	nestedManifestPath := filepath.Join(nestedDir, testManifestName)
	require.NoError(t, os.MkdirAll(nestedDir, 0o700))

	writeDirectoryManifest(t, manifestPath, nestedDir)
	writeDirectoryManifest(t, nestedManifestPath, nestedDir)

	manifest := util.NewFileManifest(root, testManifestName)
	require.NoError(t, manifest.Clean(l))

	require.NoFileExists(t, manifestPath)
	require.NoFileExists(t, nestedManifestPath)
}

func TestFileManifestCleanRemovesInvalidManifest(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	root := helpers.TmpDirWOSymlinks(t)
	manifestPath := filepath.Join(root, testManifestName)
	require.NoError(t, os.WriteFile(manifestPath, []byte("not a gob manifest\n"), 0o600))

	manifest := util.NewFileManifest(root, testManifestName)
	require.NoError(t, manifest.Clean(l))

	require.NoFileExists(t, manifestPath)
}

func writeManifest(t *testing.T, path string, paths ...string) {
	t.Helper()

	writeManifestEntries(t, path, false, paths...)
}

func writeDirectoryManifest(t *testing.T, path string, paths ...string) {
	t.Helper()

	writeManifestEntries(t, path, true, paths...)
}

func writeManifestEntries(t *testing.T, path string, isDir bool, paths ...string) {
	t.Helper()

	type entry struct {
		Path  string
		IsDir bool
	}

	f, err := os.Create(path)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, f.Close())
	}()

	enc := gob.NewEncoder(f)
	for _, p := range paths {
		require.NoError(t, enc.Encode(entry{Path: p, IsDir: isDir}))
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

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.ContainsPath(tc.path, tc.subpath)
			assert.Equal(t, tc.expected, actual, "For path %s and subpath %s", tc.path, tc.subpath)
		})
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

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.HasPathPrefix(tc.path, tc.prefix)
			assert.Equal(t, tc.expected, actual, "For path %s and prefix %s", tc.path, tc.prefix)
		})
	}
}

// copyCase is one expected include/exclude outcome for a relative path.
type copyCase struct {
	path         string
	copyExpected bool
}

// runCopyFolderContentsCase materializes the given test cases as files under
// a temp source dir, runs [util.CopyFolderContents] with the given include /
// exclude patterns, and asserts the destination matches `copyExpected`.
func runCopyFolderContentsCase(t *testing.T, includeInCopy, excludeFromCopy []string, fastCopy bool, cases []copyCase) {
	t.Helper()

	tempDir := helpers.TmpDirWOSymlinks(t)
	source := filepath.Join(tempDir, "source")
	destination := filepath.Join(tempDir, "destination")

	fileContent := []byte("source file")

	for _, tc := range cases {
		path := filepath.Join(source, tc.path)
		assert.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
		assert.NoError(t, os.WriteFile(path, fileContent, 0o644))
	}

	copyOpts := []util.CopyOption{
		util.WithIncludeInCopy(includeInCopy...),
		util.WithExcludeFromCopy(excludeFromCopy...),
	}
	if fastCopy {
		copyOpts = append(copyOpts, util.WithFastCopy())
	}

	require.NoError(t, util.CopyFolderContents(logger.CreateLogger(), source, destination, ".terragrunt-test", copyOpts...))

	for i, tc := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			_, err := os.Stat(filepath.Join(destination, tc.path))
			assert.True(t,
				tc.copyExpected && err == nil ||
					!tc.copyExpected && errors.Is(err, os.ErrNotExist),
				"Unexpected copy result for file '%s' (should be copied: '%t') - got error: %s", tc.path, tc.copyExpected, err)
		})
	}
}

func TestIncludeInCopy(t *testing.T) {
	t.Parallel()

	includeInCopy := []string{"_module/.region2", "**/app2", "**/.include-me-too"}

	cases := []copyCase{
		{path: "/app/terragrunt.hcl", copyExpected: true},
		{path: "/_module/main.tf", copyExpected: true},
		{path: "/_module/.region1/info.txt", copyExpected: false},
		{path: "/_module/.region3/project3-1/f1-2-levels.txt", copyExpected: false},
		{path: "/_module/.region3/project3-1/app1/.include-me-too/file.txt", copyExpected: true},
		{path: "/_module/.region3/project3-2/.f0/f0-3-levels.txt", copyExpected: false},
		{path: "/_module/.region2/.project2-1/app2/f2-dot-f2.txt", copyExpected: true},
		{path: "/_module/.region2/.project2-1/readme.txt", copyExpected: true},
		{path: "/_module/.region2/project2-2/f2-dot-f0.txt", copyExpected: true},
	}

	for _, mode := range []struct {
		name     string
		fastCopy bool
	}{{"slow", false}, {"fast", true}} {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			runCopyFolderContentsCase(t, includeInCopy, nil, mode.fastCopy, cases)
		})
	}
}

func TestExcludeFromCopy(t *testing.T) {
	t.Parallel()

	excludeFromCopy := []string{"module/region2", "**/exclude-me-here", "**/app1"}

	cases := []copyCase{
		{path: "/app/terragrunt.hcl", copyExpected: true},
		{path: "/module/main.tf", copyExpected: true},
		{path: "/module/region1/info.txt", copyExpected: true},
		{path: "/module/region1/project2-1/app1/f2-dot-f2.txt", copyExpected: false},
		{path: "/module/region3/project3-1/f1-2-levels.txt", copyExpected: true},
		{path: "/module/region3/project3-1/app1/exclude-me-here/file.txt", copyExpected: false},
		{path: "/module/region3/project3-2/f0/f0-3-levels.txt", copyExpected: true},
		{path: "/module/region2/project2-1/app2/f2-dot-f2.txt", copyExpected: false},
		{path: "/module/region2/project2-1/readme.txt", copyExpected: false},
		{path: "/module/region2/project2-2/f2-dot-f0.txt", copyExpected: false},
	}

	for _, mode := range []struct {
		name     string
		fastCopy bool
	}{{"slow", false}, {"fast", true}} {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			runCopyFolderContentsCase(t, nil, excludeFromCopy, mode.fastCopy, cases)
		})
	}
}

func TestExcludeFromCopyTrailingSlash(t *testing.T) {
	t.Parallel()

	excludeFromCopy := []string{"module/region2/", "**/app1/"}

	cases := []copyCase{
		{path: "/app/terragrunt.hcl", copyExpected: true},
		{path: "/module/region1/info.txt", copyExpected: true},
		{path: "/module/region1/project2-1/app1/f2-dot-f2.txt", copyExpected: false},
		{path: "/module/region2/project2-1/readme.txt", copyExpected: false},
		{path: "/module/region2/project2-2/f2-dot-f0.txt", copyExpected: false},
	}

	for _, mode := range []struct {
		name     string
		fastCopy bool
	}{{"slow", false}, {"fast", true}} {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			runCopyFolderContentsCase(t, nil, excludeFromCopy, mode.fastCopy, cases)
		})
	}
}

func TestExcludeIncludeBehaviourPriority(t *testing.T) {
	t.Parallel()

	includeInCopy := []string{"_module/.region2", "_module/.region3"}
	excludeFromCopy := []string{"**/.project2-2", "_module/.region3"}

	cases := []copyCase{
		{path: "/_module/.region2/.project2-1/app2/f2-dot-f2.txt", copyExpected: true},
		{path: "/_module/.region2/.project2-1/readme.txt", copyExpected: true},
		{path: "/_module/.region2/.project2-2/f2-dot-f0.txt", copyExpected: false},
		{path: "/_module/.region3/.project2-1/readme.txt", copyExpected: false},
	}

	for _, mode := range []struct {
		name     string
		fastCopy bool
	}{{"slow", false}, {"fast", true}} {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			runCopyFolderContentsCase(t, includeInCopy, excludeFromCopy, mode.fastCopy, cases)
		})
	}
}

func TestEmptyDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		path        string
		expectEmpty bool
	}{
		{helpers.TmpDirWOSymlinks(t), true},
		{os.TempDir(), false},
	}
	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			emptyValue, err := util.IsDirectoryEmpty(tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.expectEmpty, emptyValue, "For path %s", tc.path)
		})
	}
}

//nolint:funlen
func TestWalkWithSimpleSymlinks(t *testing.T) {
	t.Parallel()
	// Create a temporary test directory structure
	tempDir := helpers.TmpDirWOSymlinks(t)
	tempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Create directories
	dirs := []string{"a", "d"}
	for _, dir := range dirs {
		require.NoError(t, os.Mkdir(filepath.Join(tempDir, dir), 0o755))
	}

	// Create test files
	testFile := filepath.Join(tempDir, "a", "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	// Create symlinks
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "b")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "c")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "d", "a")))

	var paths []string

	err = util.WalkDirWithSymlinks(tempDir, func(path string, _ fs.DirEntry, err error) error {
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
		filepath.Join("a", "test.txt"),
		"b",
		filepath.Join("b", "test.txt"),
		"c",
		filepath.Join("c", "test.txt"),
		"d",
		filepath.Join("d", "a"),
		filepath.Join("d", "a", "test.txt"),
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
	tempDir := helpers.TmpDirWOSymlinks(t)
	tempDir, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Create directories
	dirs := []string{"a", "b", "c", "d"}
	for _, dir := range dirs {
		require.NoError(t, os.Mkdir(filepath.Join(tempDir, dir), 0o755))
	}

	// Create test files
	testFile := filepath.Join(tempDir, "a", "test.txt")
	require.NoError(t, os.WriteFile(testFile, []byte("test"), 0o644))

	// Create symlinks
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "b", "link-to-a")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "c", "another-link-to-a")))

	// Create circular symlink
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "d"), filepath.Join(tempDir, "a", "link-to-d")))
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "a"), filepath.Join(tempDir, "d", "link-to-a")))

	var paths []string

	err = util.WalkDirWithSymlinks(tempDir, func(path string, _ fs.DirEntry, err error) error {
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
		filepath.Join("a", "link-to-d"),
		filepath.Join("a", "link-to-d", "link-to-a"),
		filepath.Join("a", "link-to-d", "link-to-a", "link-to-d"),
		filepath.Join("a", "link-to-d", "link-to-a", "test.txt"),
		filepath.Join("a", "test.txt"),
		"b",
		filepath.Join("b", "link-to-a"),
		filepath.Join("b", "link-to-a", "link-to-d"),
		filepath.Join("b", "link-to-a", "test.txt"),
		"c",
		filepath.Join("c", "another-link-to-a"),
		filepath.Join("c", "another-link-to-a", "link-to-d"),
		filepath.Join("c", "another-link-to-a", "test.txt"),
		"d",
		filepath.Join("d", "link-to-a"),
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

func TestWalkDirWithSymlinksErrors(t *testing.T) {
	t.Parallel()

	tempDir := helpers.TmpDirWOSymlinks(t)

	// Test with non-existent directory
	require.Error(t, util.WalkDirWithSymlinks(filepath.Join(tempDir, "nonexistent"), func(_ string, _ fs.DirEntry, err error) error {
		return err
	}))

	// Test with broken symlink
	brokenLink := filepath.Join(tempDir, "broken")
	require.NoError(t, os.Symlink(filepath.Join(tempDir, "nonexistent"), brokenLink))

	require.Error(t, util.WalkDirWithSymlinks(tempDir, func(_ string, _ fs.DirEntry, err error) error {
		return err
	}))
}

func Test_sanitizePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseDir string
		file    string
		want    string
		wantErr bool
	}{
		{
			name:    "happy path",
			baseDir: "./testdata/fixture-sanitize-path/env/unit",
			file:    ".terraform-version",
			want:    "testdata/fixture-sanitize-path/env/unit/.terraform-version",
		},
		{
			name:    "nested file path is preserved",
			baseDir: "./testdata/fixture-sanitize-path",
			file:    "env/unit/.terraform-version",
			want:    "testdata/fixture-sanitize-path/env/unit/.terraform-version",
		},
		{
			name:    "base dir is empty",
			baseDir: "",
			file:    ".terraform-version",
			want:    "",
			wantErr: true,
		},
		{
			name:    "try to escape base dir",
			baseDir: "./testdata/fixture-sanitize-path/env/unit",
			file:    "../../../dev/random",
			want:    "",
			wantErr: true,
		},
		{
			name:    "file is empty",
			baseDir: "./testdata/fixture-sanitize-path/env/unit",
			file:    "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "file is just a slash",
			baseDir: "./testdata/fixture-sanitize-path/env/unit",
			file:    "/",
			want:    "",
			wantErr: true,
		},
		{
			name:    "file is just a dot",
			baseDir: "./testdata/fixture-sanitize-path/env/unit",
			file:    ".",
			want:    "testdata/fixture-sanitize-path/env/unit",
			wantErr: false,
		},
		{
			name:    "encoded characters",
			baseDir: "./testdata/fixture-sanitize-path/env/unit",
			file:    "..%2F..%2Fetc%2Fpasswd",
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := util.SanitizePath(tt.baseDir, tt.file)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			assert.Equalf(t, tt.want, got, "sanitizePath(%v, %v)", tt.baseDir, tt.file)
		})
	}
}

func TestMoveFile(t *testing.T) {
	t.Parallel()
	tempDir := helpers.TmpDirWOSymlinks(t)

	src := filepath.Join(tempDir, "src.txt")
	dst := filepath.Join(tempDir, "dst.txt")

	require.NoError(t, os.WriteFile(src, []byte("test"), 0o644))
	require.NoError(t, util.MoveFile(src, dst))

	// Verify the file was moved
	_, err := os.Stat(src)
	require.ErrorIs(t, err, fs.ErrNotExist)
	contents, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "test", string(contents))
}

func TestRelPathForLog(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		basePath    string
		targetPath  string
		expected    string
		showAbsPath bool
	}{
		{
			name:        "showAbsPath true returns targetPath unchanged",
			basePath:    helpers.RootFolder + "base",
			targetPath:  helpers.RootFolder + "base/child/file.txt",
			showAbsPath: true,
			expected:    helpers.RootFolder + "base/child/file.txt",
		},
		{
			name:        "same path returns targetPath",
			basePath:    helpers.RootFolder + "base",
			targetPath:  helpers.RootFolder + "base",
			showAbsPath: false,
			expected:    helpers.RootFolder + "base",
		},
		{
			name:        "child path gets ./ prefix",
			basePath:    helpers.RootFolder + "base",
			targetPath:  helpers.RootFolder + "base/child",
			showAbsPath: false,
			expected:    "." + string(filepath.Separator) + "child",
		},
		{
			name:        "nested child path gets ./ prefix",
			basePath:    helpers.RootFolder + "base",
			targetPath:  helpers.RootFolder + "base/child/subchild/file.txt",
			showAbsPath: false,
			expected:    "." + string(filepath.Separator) + filepath.Join("child", "subchild", "file.txt"),
		},
		{
			name:        "parent path returns relative path with ..",
			basePath:    helpers.RootFolder + "base/child",
			targetPath:  helpers.RootFolder + "base",
			showAbsPath: false,
			expected:    "..",
		},
		{
			name:        "sibling path returns relative path with ..",
			basePath:    helpers.RootFolder + "base/child1",
			targetPath:  helpers.RootFolder + "base/child2",
			showAbsPath: false,
			expected:    ".." + string(filepath.Separator) + "child2",
		},
		{
			name:        "deeply nested sibling path",
			basePath:    helpers.RootFolder + "base/a/b/c",
			targetPath:  helpers.RootFolder + "base/x/y/z",
			showAbsPath: false,
			expected:    ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Join("x", "y", "z"),
		},
		{
			name:        "unrelated paths at different roots",
			basePath:    helpers.RootFolder + "foo",
			targetPath:  helpers.RootFolder + "bar/baz",
			showAbsPath: false,
			expected:    ".." + string(filepath.Separator) + filepath.Join("bar", "baz"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := util.RelPathForLog(tc.basePath, tc.targetPath, tc.showAbsPath)
			assert.Equal(t, tc.expected, actual, "For basePath %s and targetPath %s", tc.basePath, tc.targetPath)
		})
	}
}

// buildCopyBenchTree lays out a synthetic module source: topDirs
// top-level directories, each a chain chainDepth levels deep with
// filesPerLevel files at every level. A bare-directory include pattern
// like the top-level name triggers the legacy expandGlobPath recursion
// once per nested directory.
//
// Returns the tree root and the top-level directory names, which the
// benchmark uses as include patterns.
func buildCopyBenchTree(b *testing.B, topDirs, chainDepth, filesPerLevel int) (string, []string) {
	b.Helper()

	root := b.TempDir()
	content := []byte("x")
	names := make([]string, 0, topDirs)

	for i := range topDirs {
		name := fmt.Sprintf("mod%03d", i)
		names = append(names, name)

		current := filepath.Join(root, name)

		for depth := range chainDepth {
			require.NoError(b, os.MkdirAll(current, 0o755))

			for f := range filesPerLevel {
				p := filepath.Join(current, fmt.Sprintf("f%02d.tf", f))
				require.NoError(b, os.WriteFile(p, content, 0o644))
			}

			current = filepath.Join(current, fmt.Sprintf("level%02d", depth))
		}
	}

	cache := filepath.Join(root, util.TerragruntCacheDir, "should-be-skipped")
	require.NoError(b, os.MkdirAll(cache, 0o755))
	require.NoError(b, os.WriteFile(filepath.Join(cache, "skip.tf"), content, 0o644))

	return root, names
}

func benchmarkCopyFolderContents(b *testing.B, fastCopy bool) {
	b.Helper()

	const (
		topDirs       = 20
		chainDepth    = 8
		filesPerLevel = 5
	)

	source, include := buildCopyBenchTree(b, topDirs, chainDepth, filesPerLevel)
	l := logger.CreateLogger()

	copyOpts := []util.CopyOption{
		util.WithIncludeInCopy(include...),
		util.WithExcludeFromCopy("**/f00.tf"),
	}
	if fastCopy {
		copyOpts = append(copyOpts, util.WithFastCopy())
	}

	for b.Loop() {
		require.NoError(b, util.CopyFolderContents(l, source, b.TempDir(), ".terragrunt-test", copyOpts...))
	}
}

func BenchmarkCopyFolderContents_Slow(b *testing.B) { benchmarkCopyFolderContents(b, false) }
func BenchmarkCopyFolderContents_Fast(b *testing.B) { benchmarkCopyFolderContents(b, true) }

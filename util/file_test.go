package util

import (
	"io/ioutil"
	"path/filepath"
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
		actual := PathContainsHiddenFileOrFolder(path)
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
	dir, err := ioutil.TempDir("", ".terragrunt-test-dir")
	require.NoError(t, err)
	for _, file := range []string{"file1", "file2"} {
		// create temp files in the dir
		f, err := ioutil.TempFile(dir, file)
		assert.NoError(t, err, f.Close())
		testfiles = append(testfiles, f.Name())
	}

	// create a manifest
	manifest := newFileManifest(dir, ".terragrunt-test-manifest")
	require.Nil(t, manifest.Create())
	// check the file manifest has been created
	require.FileExists(t, filepath.Join(manifest.ManifestFolder, manifest.ManifestFile))
	for _, file := range testfiles {
		assert.NoError(t, manifest.AddFile(file))
	}

	require.NoError(t, manifest.Clean())
	// test if the files have been deleted
	for _, file := range testfiles {
		assert.Equal(t, FileExists(file), false)
	}

}

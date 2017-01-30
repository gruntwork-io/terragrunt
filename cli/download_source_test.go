package cli

import (
	"testing"
	"fmt"
	"path/filepath"
	"github.com/stretchr/testify/assert"
	"net/url"
	"github.com/gruntwork-io/terragrunt/options"
	"io/ioutil"
	"github.com/gruntwork-io/terragrunt/util"
	"os"
)

func TestAlreadyHaveLatestCodeLocalFilePath(t *testing.T) {
	t.Parallel()

	canonicalUrl := fmt.Sprintf("file://%s", absPath(t, "../test/fixture-download-source/hello-world"))
	downloadDir := "does-not-exist"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirDoesNotExist(t *testing.T) {
	t.Parallel()

	canonicalUrl := "http://www.some-url.com"
	downloadDir := "does-not-exist"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsNoVersionNoVersionFile(t *testing.T) {
	t.Parallel()

	canonicalUrl := "http://www.some-url.com"
	downloadDir := "../test/fixture-download-source/download-dir-empty"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsNoVersionWithVersionFile(t *testing.T) {
	t.Parallel()

	canonicalUrl := "http://www.some-url.com"
	downloadDir := "../test/fixture-download-source/download-dir-version-file-no-query"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, true)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionNoVersionFile(t *testing.T) {
	t.Parallel()

	canonicalUrl := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := "../test/fixture-download-source/download-dir-empty"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionAndVersionFile(t *testing.T) {
	t.Parallel()

	canonicalUrl := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := "../test/fixture-download-source/download-dir-version-file"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, true)
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := fmt.Sprintf("file://%s", absPath(t, "../test/fixture-download-source/hello-world"))
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, "# Hello, World")
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := fmt.Sprintf("file://%s", absPath(t, "../test/fixture-download-source/hello-world"))
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, "# Hello, World")
}

// TODO re-enable these two tests once this is merged into master, since they are trying to download code from master
//func TestDownloadTerraformSourceIfNecessaryRemoteUrlToEmptyDir(t *testing.T) {
//	t.Parallel()
//
//	// TODO: update URL to master branch once this is merged
//	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world"
//	downloadDir := tmpDir(t)
//	defer os.Remove(downloadDir)
//
//	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, "# Hello, World")
//}
//
//func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDir(t *testing.T) {
//	t.Parallel()
//
//	// TODO: update URL to master branch once this is merged
//	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world"
//	downloadDir := tmpDir(t)
//	defer os.Remove(downloadDir)
//
//	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)
//
//	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, "# Hello, World 2")
//}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirDifferentVersion(t *testing.T) {
	t.Parallel()

	// TODO: update URL to master branch once this is merged
	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=cache-tmp-folder"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, "# Hello, World")
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	// TODO: update URL to master branch once this is merged
	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=cache-tmp-folder"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, "# Hello, World version remote")
}

func testDownloadTerraformSourceIfNecessary(t *testing.T, canonicalUrl string, downloadDir string, expectedFileContents string) {
	terraformSource := &TerraformSource{
		CanonicalSourceURL: parseUrl(t, canonicalUrl),
		DownloadDir: downloadDir,
		VersionFile: filepath.Join(downloadDir, "version-file.txt"),
	}

	err := downloadTerraformSourceIfNecessary(terraformSource, options.NewTerragruntOptionsForTest("./should-not-be-used"))
	assert.Nil(t, err, "For terraform source %v: %v", terraformSource, err)

	expectedFilePath := filepath.Join(downloadDir, "main.tf")
	if assert.True(t, util.FileExists(expectedFilePath), "For terraform source %v", terraformSource) {
		actualFileContents := readFile(t, expectedFilePath)
		assert.Equal(t, expectedFileContents, actualFileContents, "For terraform source %v", terraformSource)
	}
}

func testAlreadyHaveLatestCode(t *testing.T, canonicalUrl string, downloadDir string, expected bool) {
	terraformSource := &TerraformSource{
		CanonicalSourceURL: parseUrl(t, canonicalUrl),
		DownloadDir: downloadDir,
		VersionFile: filepath.Join(downloadDir, "version-file.txt"),
	}

	actual, err := alreadyHaveLatestCode(terraformSource)
	assert.Nil(t, err, "Unexpected error for terraform source %v: %v", terraformSource, err)
	assert.Equal(t, expected, actual, "For terraform source %v", terraformSource)
}

func tmpDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "download-source-test")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func absPath(t *testing.T, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func parseUrl(t *testing.T, rawUrl string) *url.URL {
	parsed, err := url.Parse(rawUrl)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func readFile(t *testing.T, path string) string {
	contents, err := util.ReadFileAsString(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func copyFolder(t *testing.T, src string, dest string) {
	err := util.CopyFolderContents(src, dest)
	if err != nil {
		t.Fatal(err)
	}
}
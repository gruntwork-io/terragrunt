package cli

import (
	"fmt"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"testing"
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

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World")
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := fmt.Sprintf("file://%s", absPath(t, "../test/fixture-download-source/hello-world"))
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World")
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World")
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World 2")
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirDifferentVersion(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World")
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World version remote")
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlOverrideSource(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, true, "# Hello, World")
}

func testDownloadTerraformSourceIfNecessary(t *testing.T, canonicalUrl string, downloadDir string, sourceUpdate bool, expectedFileContents string) {
	terraformSource := &TerraformSource{
		CanonicalSourceURL: parseUrl(t, canonicalUrl),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        util.JoinPath(downloadDir, "version-file.txt"),
	}

	terragruntOptions := options.NewTerragruntOptionsForTest("./should-not-be-used")
	terragruntOptions.SourceUpdate = sourceUpdate
	terragruntConfig := config.TerragruntConfig{}

	err := downloadTerraformSourceIfNecessary(terraformSource, &terragruntConfig, terragruntOptions)
	assert.Nil(t, err, "For terraform source %v: %v", terraformSource, err)

	expectedFilePath := util.JoinPath(downloadDir, "main.tf")
	if assert.True(t, util.FileExists(expectedFilePath), "For terraform source %v", terraformSource) {
		actualFileContents := readFile(t, expectedFilePath)
		assert.Equal(t, expectedFileContents, actualFileContents, "For terraform source %v", terraformSource)
	}
}

func testAlreadyHaveLatestCode(t *testing.T, canonicalUrl string, downloadDir string, expected bool) {
	terraformSource := &TerraformSource{
		CanonicalSourceURL: parseUrl(t, canonicalUrl),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        util.JoinPath(downloadDir, "version-file.txt"),
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

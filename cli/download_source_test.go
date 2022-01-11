package cli

import (
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/errors"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/cli/tfsource"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
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

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionAndVersionFileAndTfCode(t *testing.T) {
	t.Parallel()

	canonicalUrl := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := "../test/fixture-download-source/download-dir-version-file-tf-code"

	testAlreadyHaveLatestCode(t, canonicalUrl, downloadDir, true)
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := fmt.Sprintf("file://%s", absPath(t, "../test/fixture-download-source/hello-world"))
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := fmt.Sprintf("file://%s", absPath(t, "../test/fixture-download-source/hello-world"))
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World 2", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirDifferentVersion(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, false, "# Hello, World version remote", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlOverrideSource(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalUrl, downloadDir, true, "# Hello, World", true)
}

func TestInvalidModulePath(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world-version-remote/not-existing-path?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	terraformSource, _, _, err := createConfig(t, canonicalUrl, downloadDir, false)
	assert.Nil(t, err)
	terraformSource.WorkingDir = terraformSource.WorkingDir + "/not-existing-path"

	err = validateWorkingDir(terraformSource)
	assert.NotNil(t, err)
	_, ok := errors.Unwrap(err).(WorkingDirNotFound)
	assert.True(t, ok)
}

func TestDownloadInvalidPathToFilePath(t *testing.T) {
	t.Parallel()

	canonicalUrl := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world/main.tf?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../test/fixture-download-source/hello-world-version-remote", downloadDir)

	terraformSource, _, _, err := createConfig(t, canonicalUrl, downloadDir, false)
	assert.Nil(t, err)
	terraformSource.WorkingDir = terraformSource.WorkingDir + "/main.tf"

	err = validateWorkingDir(terraformSource)
	assert.NotNil(t, err)
	_, ok := errors.Unwrap(err).(WorkingDirNotDir)
	assert.True(t, ok)
}

func TestDownloadTerraformSourceFromLocalFolderWithManifest(t *testing.T) {
	t.Parallel()

	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	// used to test if an empty folder gets copied
	testDir := tmpDir(t)
	require.NoError(t, os.Mkdir(path.Join(testDir, "sub2"), 0700))
	defer os.Remove(testDir)

	testCases := []struct {
		name      string
		sourceURL string
		comp      assert.Comparison
	}{
		{
			"test-stale-file-exists", "../test/fixture-manifest/version-1",
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "stale.tf"))
			},
		},
		{
			"test-stale-file-doesnt-exist-after-source-update", "../test/fixture-manifest/version-2",
			func() bool {
				return !util.FileExists(filepath.Join(downloadDir, "stale.tf"))
			},
		},
		{
			"test-tffile-exists-in-subfolder", "../test/fixture-manifest/version-3-subfolder",
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub", "main.tf"))
			},
		},
		{
			"test-tffile-doesnt-exist-in-subfolder", "../test/fixture-manifest/version-4-subfolder-empty",
			func() bool {
				return !util.FileExists(filepath.Join(downloadDir, "sub", "main.tf"))
			},
		},
		{
			"test-empty-folder-gets-copied", testDir,
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub2"))
			},
		},
		{
			"test-empty-folder-gets-populated", "../test/fixture-manifest/version-5-not-empty-subfolder",
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub2", "main.tf"))
			},
		},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			copyFolder(t, testCase.sourceURL, downloadDir)
			require.Condition(t, testCase.comp)
		})

	}

}

func testDownloadTerraformSourceIfNecessary(t *testing.T, canonicalUrl string, downloadDir string, sourceUpdate bool, expectedFileContents string, requireInitFile bool) {
	terraformSource, terragruntOptions, terragruntConfig, err := createConfig(t, canonicalUrl, downloadDir, sourceUpdate)

	err = downloadTerraformSourceIfNecessary(terraformSource, terragruntOptions, terragruntConfig)
	require.NoError(t, err, "For terraform source %v: %v", terraformSource, err)

	expectedFilePath := util.JoinPath(downloadDir, "main.tf")
	if assert.True(t, util.FileExists(expectedFilePath), "For terraform source %v", terraformSource) {
		actualFileContents := readFile(t, expectedFilePath)
		assert.Equal(t, expectedFileContents, actualFileContents, "For terraform source %v", terraformSource)
	}

	if requireInitFile {
		existsInitFile := util.FileExists(util.JoinPath(terraformSource.WorkingDir, moduleInitRequiredFile))
		assert.True(t, existsInitFile)
	}
}

func createConfig(t *testing.T, canonicalUrl string, downloadDir string, sourceUpdate bool) (*tfsource.TerraformSource, *options.TerragruntOptions, *config.TerragruntConfig, error) {
	terraformSource := &tfsource.TerraformSource{
		CanonicalSourceURL: parseUrl(t, canonicalUrl),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        util.JoinPath(downloadDir, "version-file.txt"),
	}

	terragruntOptions, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	terragruntOptions.SourceUpdate = sourceUpdate
	terragruntOptions.Env = parseEnvironmentVariables(os.Environ())

	terragruntConfig := &config.TerragruntConfig{
		Terraform: &config.TerraformConfig{
			ExtraArgs: []config.TerraformExtraArguments{},
			Source:    nil,
		},
	}

	err = PopulateTerraformVersion(terragruntOptions)
	assert.Nil(t, err, "For terraform source %v: %v", terraformSource, err)
	return terraformSource, terragruntOptions, terragruntConfig, err
}

func testAlreadyHaveLatestCode(t *testing.T, canonicalUrl string, downloadDir string, expected bool) {
	terraformSource := &tfsource.TerraformSource{
		CanonicalSourceURL: parseUrl(t, canonicalUrl),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        util.JoinPath(downloadDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	assert.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	actual, err := alreadyHaveLatestCode(terraformSource, opts)
	assert.Nil(t, err, "Unexpected error for terraform source %v: %v", terraformSource, err)
	assert.Equal(t, expected, actual, "For terraform source %v", terraformSource)
}

func tmpDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "download-source-test")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.FromSlash(dir)
}

func absPath(t *testing.T, path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func parseUrl(t *testing.T, str string) *url.URL {
	// URLs should have only forward slashes, whereas on Windows, the file paths may be backslashes
	rawUrl := strings.Join(strings.Split(str, string(filepath.Separator)), "/")

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
	err := util.CopyFolderContents(filepath.FromSlash(src), filepath.FromSlash(dest), ".terragrunt-test", nil)
	require.Nil(t, err)
}

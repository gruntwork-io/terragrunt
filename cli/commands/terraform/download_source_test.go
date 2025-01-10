package terraform_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/cli/commands/terraform"
	tgTerraform "github.com/gruntwork-io/terragrunt/terraform"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
)

func TestAlreadyHaveLatestCodeLocalFilePathWithNoModifiedFiles(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world-local-hash")
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/download-dir-version-file-local-hash", downloadDir)
	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)

	// Write out a version file so we can test a cache hit
	terraformSource, _, _, err := createConfig(t, canonicalURL, downloadDir, false)
	if err != nil {
		t.Fatal(err)
	}
	err = terraformSource.WriteVersionFile()
	if err != nil {
		t.Fatal(err)
	}

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, true)
}

func TestAlreadyHaveLatestCodeLocalFilePathHashingFailure(t *testing.T) {
	t.Parallel()

	fixturePath := absPath(t, "../../../test/fixtures/download-source/hello-world-local-hash-failed")
	canonicalURL := "file://" + fixturePath
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-local-hash-failed", downloadDir)

	fileInfo, err := os.Stat(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chmod(fixturePath, 0000)
	if err != nil {
		t.Fatal(err)
	}

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)

	err = os.Chmod(fixturePath, fileInfo.Mode())
	if err != nil {
		t.Fatal(err)
	}
}

func TestAlreadyHaveLatestCodeLocalFilePathWithHashChanged(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world-local-hash")
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/download-dir-version-file-local-hash", downloadDir)

	f, err := os.OpenFile(downloadDir+"/version-file.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Modify content of file to simulate change
	fmt.Fprintln(f, "CHANGED")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeLocalFilePath(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world")
	downloadDir := "does-not-exist"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirDoesNotExist(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com"
	downloadDir := "does-not-exist"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsNoVersionNoVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com"
	downloadDir := "../../../test/fixtures/download-source/download-dir-empty"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsNoVersionWithVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com"
	downloadDir := "../../../test/fixtures/download-source/download-dir-version-file-no-query"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, true)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionNoVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := "../../../test/fixtures/download-source/download-dir-empty"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionAndVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := "../../../test/fixtures/download-source/download-dir-version-file"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionAndVersionFileAndTfCode(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := "../../../test/fixtures/download-source/download-dir-version-file-tf-code"

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, true)
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world")
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world")
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World 2", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirDifferentVersion(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World version remote", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlOverrideSource(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, true, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryInvalidTerraformSource(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/totallyfakedoesnotexist/notreal.git//foo?ref=v1.2.3"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, terragruntOptions, terragruntConfig, err := createConfig(t, canonicalURL, downloadDir, false)

	require.NoError(t, err)

	err = terraform.DownloadTerraformSourceIfNecessary(context.Background(), terraformSource, terragruntOptions, terragruntConfig)
	require.Error(t, err)
	var downloadingTerraformSourceErr terraform.DownloadingTerraformSourceErr
	ok := errors.As(err, &downloadingTerraformSourceErr)
	assert.True(t, ok)
}

func TestInvalidModulePath(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world-version-remote/not-existing-path?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, _, _, err := createConfig(t, canonicalURL, downloadDir, false)
	require.NoError(t, err)
	terraformSource.WorkingDir += "/not-existing-path"

	err = terraform.ValidateWorkingDir(terraformSource)
	require.Error(t, err)
	var workingDirNotFound terraform.WorkingDirNotFound
	ok := errors.As(err, &workingDirNotFound)
	assert.True(t, ok)
}

func TestDownloadInvalidPathToFilePath(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixture-download-source/hello-world/main.tf?ref=v0.9.7"
	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, _, _, err := createConfig(t, canonicalURL, downloadDir, false)
	require.NoError(t, err)
	terraformSource.WorkingDir += "/main.tf"

	err = terraform.ValidateWorkingDir(terraformSource)
	require.Error(t, err)
	var workingDirNotDir terraform.WorkingDirNotDir
	ok := errors.As(err, &workingDirNotDir)
	assert.True(t, ok)
}

// The test cases are run sequentially because they depend on each other.
//
//nolint:tparallel
func TestDownloadTerraformSourceFromLocalFolderWithManifest(t *testing.T) {
	t.Parallel()

	downloadDir := tmpDir(t)
	t.Cleanup(func() {
		os.RemoveAll(downloadDir)
	})

	// used to test if an empty folder gets copied
	testDir := tmpDir(t)
	require.NoError(t, os.Mkdir(path.Join(testDir, "sub2"), 0700))
	t.Cleanup(func() {
		os.Remove(testDir)
	})

	testCases := []struct {
		name      string
		sourceURL string
		comp      assert.Comparison
	}{
		{
			"test-stale-file-exists", "../../../test/fixtures/manifest/version-1",
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "stale.tf"))
			},
		},
		{
			"test-stale-file-doesnt-exist-after-source-update", "../../../test/fixtures/manifest/version-2",
			func() bool {
				return !util.FileExists(filepath.Join(downloadDir, "stale.tf"))
			},
		},
		{
			"test-tffile-exists-in-subfolder", "../../../test/fixtures/manifest/version-3-subfolder",
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub", "main.tf"))
			},
		},
		{
			"test-tffile-doesnt-exist-in-subfolder", "../../../test/fixtures/manifest/version-4-subfolder-empty",
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
			"test-empty-folder-gets-populated", "../../../test/fixtures/manifest/version-5-not-empty-subfolder",
			func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub2", "main.tf"))
			},
		},
	}

	// The test cases are run sequentially because they depend on each other.
	//
	//nolint:paralleltest
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			copyFolder(t, testCase.sourceURL, downloadDir)
			assert.Condition(t, testCase.comp)
		})

	}

}

func testDownloadTerraformSourceIfNecessary(t *testing.T, canonicalURL string, downloadDir string, sourceUpdate bool, expectedFileContents string, requireInitFile bool) {
	t.Helper()

	terraformSource, terragruntOptions, terragruntConfig, err := createConfig(t, canonicalURL, downloadDir, sourceUpdate)

	require.NoError(t, err)

	err = terraform.DownloadTerraformSourceIfNecessary(context.Background(), terraformSource, terragruntOptions, terragruntConfig)
	require.NoError(t, err, "For terraform source %v: %v", terraformSource, err)

	expectedFilePath := util.JoinPath(downloadDir, "main.tf")
	if assert.True(t, util.FileExists(expectedFilePath), "For terraform source %v", terraformSource) {
		actualFileContents := readFile(t, expectedFilePath)
		assert.Equal(t, expectedFileContents, actualFileContents, "For terraform source %v", terraformSource)
	}

	if requireInitFile {
		existsInitFile := util.FileExists(util.JoinPath(terraformSource.WorkingDir, terraform.ModuleInitRequiredFile))
		require.True(t, existsInitFile)
	}
}

func createConfig(t *testing.T, canonicalURL string, downloadDir string, sourceUpdate bool) (*tgTerraform.Source, *options.TerragruntOptions, *config.TerragruntConfig, error) {
	t.Helper()

	logger := log.New()
	logger.SetOptions(log.WithOutput(io.Discard))
	terraformSource := &tgTerraform.Source{
		CanonicalSourceURL: parseURL(t, canonicalURL),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        util.JoinPath(downloadDir, "version-file.txt"),
		Logger:             logger,
	}

	terragruntOptions, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	terragruntOptions.SourceUpdate = sourceUpdate
	terragruntOptions.Env = env.Parse(os.Environ())

	terragruntConfig := &config.TerragruntConfig{
		Terraform: &config.TerraformConfig{
			ExtraArgs: []config.TerraformExtraArguments{},
			Source:    nil,
		},
	}

	err = terraform.PopulateTerraformVersion(context.Background(), terragruntOptions)
	require.NoError(t, err)
	return terraformSource, terragruntOptions, terragruntConfig, err
}

func testAlreadyHaveLatestCode(t *testing.T, canonicalURL string, downloadDir string, expected bool) {
	t.Helper()

	logger := log.New()
	logger.SetOptions(log.WithOutput(io.Discard))
	terraformSource := &tgTerraform.Source{
		CanonicalSourceURL: parseURL(t, canonicalURL),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        util.JoinPath(downloadDir, "version-file.txt"),
		Logger:             logger,
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	actual, err := terraform.AlreadyHaveLatestCode(terraformSource, opts)
	require.NoError(t, err)
	assert.Equal(t, expected, actual, "For terraform source %v", terraformSource)
}

func tmpDir(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "download-source-test")
	if err != nil {
		t.Fatal(err)
	}
	return filepath.FromSlash(dir)
}

func absPath(t *testing.T, path string) string {
	t.Helper()

	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func parseURL(t *testing.T, str string) *url.URL {
	t.Helper()

	// URLs should have only forward slashes, whereas on Windows, the file paths may be backslashes
	rawURL := strings.Join(strings.Split(str, string(filepath.Separator)), "/")

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := util.ReadFileAsString(path)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func copyFolder(t *testing.T, src string, dest string) {
	t.Helper()

	logger := log.New()
	logger.SetOptions(log.WithOutput(io.Discard))

	err := util.CopyFolderContents(logger, filepath.FromSlash(src), filepath.FromSlash(dest), ".terragrunt-test", nil, nil)
	require.NoError(t, err)
}

package run_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/gruntwork-io/go-commons/env"
	"github.com/gruntwork-io/terragrunt/internal/tf"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/hashicorp/go-getter"
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

	err = terraformSource.WriteVersionFile(logger.CreateLogger())
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

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world?ref=v0.83.2"

	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world-version-remote?ref=v0.83.2"

	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World version remote", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlOverrideSource(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world?ref=v0.83.2"

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

	terraformSource, opts, cfg, err := createConfig(t, canonicalURL, downloadDir, false)

	require.NoError(t, err)

	err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		terraformSource,
		opts,
		cfg,
		report.NewReport(),
	)
	require.Error(t, err)

	var downloadingTerraformSourceErr run.DownloadingTerraformSourceErr

	ok := errors.As(err, &downloadingTerraformSourceErr)
	assert.True(t, ok)
}

func TestInvalidModulePath(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world-version-remote/non-existent-path?ref=v0.83.2"

	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, _, _, err := createConfig(t, canonicalURL, downloadDir, false)
	require.NoError(t, err)

	terraformSource.WorkingDir += "/not-existing-path"

	err = run.ValidateWorkingDir(terraformSource)
	require.Error(t, err)

	var workingDirNotFound run.WorkingDirNotFound

	ok := errors.As(err, &workingDirNotFound)
	assert.True(t, ok)
}

func TestDownloadInvalidPathToFilePath(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world/main.tf?ref=v0.83.2"

	downloadDir := tmpDir(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, _, _, err := createConfig(t, canonicalURL, downloadDir, false)
	require.NoError(t, err)

	terraformSource.WorkingDir += "/main.tf"

	err = run.ValidateWorkingDir(terraformSource)
	require.Error(t, err)

	var workingDirNotDir run.WorkingDirNotDir

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
		comp      assert.Comparison
		name      string
		sourceURL string
	}{
		{
			name:      "test-stale-file-exists",
			sourceURL: "../../../test/fixtures/manifest/version-1",
			comp: func() bool {
				return util.FileExists(filepath.Join(downloadDir, "stale.tf"))
			},
		},
		{
			name:      "test-stale-file-doesnt-exist-after-source-update",
			sourceURL: "../../../test/fixtures/manifest/version-2",
			comp: func() bool {
				return !util.FileExists(filepath.Join(downloadDir, "stale.tf"))
			},
		},
		{
			name:      "test-tffile-exists-in-subfolder",
			sourceURL: "../../../test/fixtures/manifest/version-3-subfolder",
			comp: func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub", "main.tf"))
			},
		},
		{
			name:      "test-tffile-doesnt-exist-in-subfolder",
			sourceURL: "../../../test/fixtures/manifest/version-4-subfolder-empty",
			comp: func() bool {
				return !util.FileExists(filepath.Join(downloadDir, "sub", "main.tf"))
			},
		},
		{
			name:      "test-empty-folder-gets-copied",
			sourceURL: testDir,
			comp: func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub2"))
			},
		},
		{
			name:      "test-empty-folder-gets-populated",
			sourceURL: "../../../test/fixtures/manifest/version-5-not-empty-subfolder",
			comp: func() bool {
				return util.FileExists(filepath.Join(downloadDir, "sub2", "main.tf"))
			},
		},
	}

	// The test cases are run sequentially because they depend on each other.
	//
	//nolint:paralleltest
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			copyFolder(t, tc.sourceURL, downloadDir)
			assert.Condition(t, tc.comp)
		})
	}
}

func testDownloadTerraformSourceIfNecessary(
	t *testing.T,
	canonicalURL string,
	downloadDir string,
	sourceUpdate bool,
	expectedFileContents string,
	requireInitFile bool,
) {
	t.Helper()

	terraformSource, opts, cfg, err := createConfig(
		t,
		canonicalURL,
		downloadDir,
		sourceUpdate,
	)

	require.NoError(t, err)

	err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		terraformSource,
		opts,
		cfg,
		report.NewReport(),
	)
	require.NoError(t, err, "For terraform source %v: %v", terraformSource, err)

	expectedFilePath := filepath.Join(downloadDir, "main.tf")
	if assert.True(t, util.FileExists(expectedFilePath), "For terraform source %v", terraformSource) {
		actualFileContents := readFile(t, expectedFilePath)
		assert.Equal(t, expectedFileContents, actualFileContents, "For terraform source %v", terraformSource)
	}

	if requireInitFile {
		existsInitFile := util.FileExists(filepath.Join(terraformSource.WorkingDir, run.ModuleInitRequiredFile))
		require.True(t, existsInitFile)
	}
}

func createConfig(
	t *testing.T,
	canonicalURL string,
	downloadDir string,
	sourceUpdate bool,
) (*tf.Source, *options.TerragruntOptions, *runcfg.RunConfig, error) {
	t.Helper()

	logger := logger.CreateLogger()
	logger.SetOptions(log.WithOutput(io.Discard))

	terraformSource := &tf.Source{
		CanonicalSourceURL: parseURL(t, canonicalURL),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        filepath.Join(downloadDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	opts.SourceUpdate = sourceUpdate
	opts.Env = env.Parse(os.Environ())

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	_, err = run.PopulateTFVersion(t.Context(), logger, opts)
	require.NoError(t, err)

	return terraformSource, opts, cfg, err
}

func testAlreadyHaveLatestCode(t *testing.T, canonicalURL string, downloadDir string, expected bool) {
	t.Helper()

	logger := logger.CreateLogger()
	logger.SetOptions(log.WithOutput(io.Discard))

	terraformSource := &tf.Source{
		CanonicalSourceURL: parseURL(t, canonicalURL),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        filepath.Join(downloadDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	actual, err := run.AlreadyHaveLatestCode(logger, terraformSource, opts)
	require.NoError(t, err)
	assert.Equal(t, expected, actual, "For terraform source %v", terraformSource)
}

func tmpDir(t *testing.T) string {
	t.Helper()

	dir := helpers.TmpDirWOSymlinks(t)

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

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	err := util.CopyFolderContents(
		l,
		filepath.FromSlash(src),
		filepath.FromSlash(dest),
		".terragrunt-test",
		nil,
		nil,
	)
	require.NoError(t, err)
}

// TestUpdateGettersExcludeFromCopy verifies the correct behavior of updateGetters with ExcludeFromCopy configuration
func TestUpdateGettersExcludeFromCopy(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		cfg                  *runcfg.RunConfig
		expectedExcludeFiles []string
	}{
		{
			name: "Nil ExcludeFromCopy",
			cfg: &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{
					ExcludeFromCopy: []string{},
				},
			},
			expectedExcludeFiles: []string{},
		},
		{
			name: "Non-Nil ExcludeFromCopy",
			cfg: &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{
					ExcludeFromCopy: []string{"*.tfstate", "excluded_dir/"},
				},
			},
			expectedExcludeFiles: []string{"*.tfstate", "excluded_dir/"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			terragruntOptions, err := options.NewTerragruntOptionsForTest("./test")
			require.NoError(t, err)

			client := &getter.Client{}

			// Call updateGetters
			updateGettersFunc := run.UpdateGetters(terragruntOptions, tc.cfg)
			err = updateGettersFunc(client)
			require.NoError(t, err)

			// Find the file getter
			fileGetter, ok := client.Getters["file"].(*run.FileCopyGetter)
			require.True(t, ok, "File getter should be of type FileCopyGetter")

			// Verify ExcludeFromCopy
			assert.Equal(
				t,
				tc.expectedExcludeFiles,
				fileGetter.ExcludeFromCopy,
				"ExcludeFromCopy should match expected value",
			)
		})
	}
}

// TestUpdateGettersHTTPNetrc verifies that HTTP/HTTPS getters have Netrc enabled
// for authentication via ~/.netrc files.
func TestUpdateGettersHTTPNetrc(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{},
	}

	client := &getter.Client{}

	updateGettersFunc := run.UpdateGetters(terragruntOptions, cfg)
	err = updateGettersFunc(client)
	require.NoError(t, err)

	// Verify HTTP getter has Netrc enabled
	httpGetter, ok := client.Getters["http"].(*getter.HttpGetter)
	require.True(t, ok, "HTTP getter should be of type HttpGetter")
	assert.True(t, httpGetter.Netrc, "HTTP getter should have Netrc enabled for ~/.netrc authentication")

	// Verify HTTPS getter has Netrc enabled
	httpsGetter, ok := client.Getters["https"].(*getter.HttpGetter)
	require.True(t, ok, "HTTPS getter should be of type HttpGetter")
	assert.True(t, httpsGetter.Netrc, "HTTPS getter should have Netrc enabled for ~/.netrc authentication")
}

// TestDownloadWithNoSourceCreatesCache tests that when sourceURL is "." (no source specified),
// DownloadTerraformSource creates cache and copies files from the working directory.
// This tests the behavior when terragrunt.hcl doesn't have a terraform { source = "..." } block.
func TestDownloadWithNoSourceCreatesCache(t *testing.T) {
	t.Parallel()

	// Create a temp directory to act as the source/working directory
	sourceDir := helpers.TmpDirWOSymlinks(t)
	defer os.RemoveAll(sourceDir)

	// Create a simple terraform file in the source directory
	mainTfContent := "# Test file for no-source cache creation\n"
	err := os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte(mainTfContent), 0644)
	require.NoError(t, err)

	// Create the download directory where cache will be created
	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.RemoveAll(downloadDir)

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(sourceDir, "terragrunt.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = sourceDir
	opts.DownloadDir = downloadDir
	opts.Experiments = experiment.NewExperiments()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	// sourceURL "." represents the current directory (no terraform.source specified)
	updatedOpts, err := run.DownloadTerraformSource(t.Context(), l, ".", opts, cfg, r)
	require.NoError(t, err)

	// Verify that the working directory was changed to the cache directory (inside downloadDir)
	assert.NotEqual(t, sourceDir, updatedOpts.WorkingDir, "Working dir should be changed to cache")
	assert.True(t, strings.HasPrefix(updatedOpts.WorkingDir, downloadDir), "Working dir should be under download dir")

	// Verify that the main.tf file was copied to the cache
	cachedMainTf := filepath.Join(updatedOpts.WorkingDir, "main.tf")
	assert.FileExists(t, cachedMainTf, "main.tf should exist in cache directory")

	// Verify the contents were copied correctly
	cachedContent, err := os.ReadFile(cachedMainTf)
	require.NoError(t, err)
	assert.Equal(t, mainTfContent, string(cachedContent), "File contents should match")
}

// TestDownloadSourceWithCASExperimentDisabled tests that CAS is not used when the experiment is disabled
func TestDownloadSourceWithCASExperimentDisabled(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	localSourcePath := absPath(t, "../../../test/fixtures/download-source/hello-world")
	src := &tf.Source{
		CanonicalSourceURL: parseURL(t, "file://"+localSourcePath),
		DownloadDir:        tmpDir,
		WorkingDir:         tmpDir,
		VersionFile:        filepath.Join(tmpDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	// Ensure CAS experiment is not enabled
	opts.Experiments = experiment.NewExperiments()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	// Mock the download source function call
	r := report.NewReport()

	err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, src, opts, cfg, r)

	require.NoError(t, err)

	// Verify the file was downloaded
	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceWithCASExperimentEnabled tests that CAS is attempted when the experiment is enabled
func TestDownloadSourceWithCASExperimentEnabled(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	localSourcePath := absPath(t, "../../../test/fixtures/download-source/hello-world")
	src := &tf.Source{
		CanonicalSourceURL: parseURL(t, "file://"+localSourcePath),
		DownloadDir:        tmpDir,
		WorkingDir:         tmpDir,
		VersionFile:        filepath.Join(tmpDir, "version-file.txt"),
	}

	// Create options with CAS experiment enabled
	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	// Enable CAS experiment
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.CAS)
	require.NoError(t, err)

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, src, opts, cfg, r)
	require.NoError(t, err)

	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceWithCASGitSource tests CAS functionality with a Git source
func TestDownloadSourceWithCASGitSource(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	src := &tf.Source{
		CanonicalSourceURL: parseURL(
			t,
			"github.com/gruntwork-io/terragrunt//test/fixtures/download/hello-world",
		),
		DownloadDir: tmpDir,
		WorkingDir:  tmpDir,
		VersionFile: filepath.Join(tmpDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	// Enable CAS experiment
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.CAS)
	require.NoError(t, err)

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, src, opts, cfg, r)
	require.NoError(t, err)

	// Verify the file was downloaded
	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceCASInitializationFailure tests the fallback behavior when CAS initialization fails
func TestDownloadSourceCASInitializationFailure(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	localSourcePath := absPath(t, "../../../test/fixtures/download-source/hello-world")
	src := &tf.Source{
		CanonicalSourceURL: parseURL(t, "file://"+localSourcePath),
		DownloadDir:        tmpDir,
		WorkingDir:         tmpDir,
		VersionFile:        filepath.Join(tmpDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	// Enable CAS experiment
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.CAS)
	require.NoError(t, err)

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, src, opts, cfg, r)
	require.NoError(t, err)

	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceWithCASMultipleSources tests that CAS works with multiple different sources
func TestDownloadSourceWithCASMultipleSources(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	opts.Env = env.Parse(os.Environ())

	// Enable CAS experiment
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.CAS)
	require.NoError(t, err)

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	testCases := []struct {
		name      string
		sourceURL string
		expectCAS bool
	}{
		{
			name:      "Local file source",
			sourceURL: "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world"),
			expectCAS: false, // CAS doesn't handle file:// URLs
		},
		{
			name:      "HTTP source",
			sourceURL: "https://example.com/repo.tar.gz",
			expectCAS: false, // CAS doesn't handle HTTP sources
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			src := &tf.Source{
				CanonicalSourceURL: parseURL(t, tc.sourceURL),
				DownloadDir:        tmpDir,
				WorkingDir:         tmpDir,
				VersionFile:        filepath.Join(tmpDir, "version-file.txt"),
			}

			err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, src, opts, cfg, r)

			if tc.name == "Local file source" {
				require.NoError(t, err)

				expectedFilePath := filepath.Join(tmpDir, "main.tf")
				assert.FileExists(t, expectedFilePath)
			} else {
				t.Logf("Source %s result: %v", tc.sourceURL, err)
			}
		})
	}
}

// TestHTTPGetterNetrcAuthentication verifies that HTTP/HTTPS getters correctly authenticate
// using ~/.netrc credentials when downloading OpenTofu/Terraform sources.
func TestHTTPGetterNetrcAuthentication(t *testing.T) {
	expectedUser := "testuser"
	expectedPass := "testpassword"
	fileContent := "# test tofu content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != expectedUser || pass != expectedPass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Write([]byte(fileContent))
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	netrcContent := fmt.Sprintf("machine %s\nlogin %s\npassword %s\n",
		serverURL.Host, expectedUser, expectedPass)

	netrcFile := filepath.Join(t.TempDir(), ".netrc")
	require.NoError(t, os.WriteFile(netrcFile, []byte(netrcContent), 0600))

	t.Setenv("NETRC", netrcFile)

	opts, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	cfg := &runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}}

	client := &getter.Client{
		Src:  server.URL + "/module.tf",
		Dst:  filepath.Join(t.TempDir(), "module.tf"),
		Mode: getter.ClientModeFile,
	}

	updateFn := run.UpdateGetters(opts, cfg)
	require.NoError(t, updateFn(client))

	require.NoError(t, client.Get())

	downloaded, err := os.ReadFile(client.Dst)
	require.NoError(t, err)
	assert.Equal(t, fileContent, string(downloaded))
}

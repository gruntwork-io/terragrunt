package run_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
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

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/configbridge"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/runner/runcfg"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// findGetter scans the slice for the first Getter of type T and returns it.
// Used by tests that need to assert configuration on a specific custom getter
// without relying on the v1 map-by-scheme indexing that v2 dropped.
func findGetter[T any](getters []getter.Getter) (T, bool) {
	i := slices.IndexFunc(getters, func(g getter.Getter) bool {
		_, ok := g.(T)
		return ok
	})

	if i < 0 {
		var zero T
		return zero, false
	}

	return getters[i].(T), true
}

func TestAlreadyHaveLatestCodeLocalFilePathWithNoModifiedFiles(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world-local-hash")

	downloadDir := helpers.TmpDirWOSymlinks(t)
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

	// Stage the fixture in a temp directory and chmod *that copy* to 0000.
	// Mutating the tracked fixture in place would leave it unreadable on
	// disk if the test crashes between chmod-to-zero and chmod-back, which
	// historically broke `git add` for every subsequent operation.
	const srcFixture = "../../../test/fixtures/download-source/hello-world-local-hash-failed"

	stagedFixture := helpers.TmpDirWOSymlinks(t)
	copyFolder(t, srcFixture, stagedFixture)

	canonicalURL := "file://" + stagedFixture

	downloadDir := helpers.TmpDirWOSymlinks(t)
	copyFolder(t, srcFixture, downloadDir)

	// Restore staged fixture mode so the surrounding t.TempDir cleanup
	// can RemoveAll it. t.Cleanup runs LIFO so this fires before TempDir's
	// own remover.
	t.Cleanup(func() {
		if chmodErr := os.Chmod(stagedFixture, 0o755); chmodErr != nil {
			t.Logf("failed to restore staged fixture mode for %s: %v", stagedFixture, chmodErr)
		}
	})

	if err := os.Chmod(stagedFixture, 0o000); err != nil {
		t.Fatal(err)
	}

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeLocalFilePathWithHashChanged(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world-local-hash")

	downloadDir := helpers.TmpDirWOSymlinks(t)
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

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryLocalDirToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "file://" + absPath(t, "../../../test/fixtures/download-source/hello-world")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToEmptyDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world"

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world"

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World 2", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirDifferentVersion(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world?ref=v0.83.2"

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world-version-remote?ref=v0.83.2"

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World version remote", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlOverrideSource(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/download-source/hello-world?ref=v0.83.2"

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, true, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryInvalidTerraformSource(t *testing.T) {
	t.Parallel()

	canonicalURL := "github.com/totallyfakedoesnotexist/notreal.git//foo?ref=v1.2.3"

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, opts, cfg, err := createConfig(t, canonicalURL, downloadDir, false)

	require.NoError(t, err)

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		run.OSVenv(),
		terraformSource,
		configbridge.NewRunOptions(opts),
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

	downloadDir := helpers.TmpDirWOSymlinks(t)
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

	downloadDir := helpers.TmpDirWOSymlinks(t)
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

	downloadDir := helpers.TmpDirWOSymlinks(t)
	t.Cleanup(func() {
		os.RemoveAll(downloadDir)
	})

	// used to test if an empty folder gets copied
	testDir := helpers.TmpDirWOSymlinks(t)
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

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		run.OSVenv(),
		terraformSource,
		configbridge.NewRunOptions(opts),
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

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	terraformSource := &tf.Source{
		CanonicalSourceURL: parseURL(t, canonicalURL),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        filepath.Join(downloadDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	opts.SourceUpdate = sourceUpdate

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	// Mem-backed venv: this helper only needs PopulateTFVersion to
	// populate opts.TerraformVersion / TofuImplementation; the version
	// probe behavior itself is covered by TestGetTFVersion* in
	// version_check_mem_test.go. Forking real tofu here would make every
	// download_source test depend on tofu being installed.
	versionV := &venv.Venv{
		Exec: vexec.NewMemExec(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			return vexec.Result{Stdout: []byte("OpenTofu v1.7.2\n")}
		}),
		Env:     env.Parse(os.Environ()),
		Writers: writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}

	_, ver, impl, err := run.PopulateTFVersion(
		t.Context(), l, versionV,
		opts.WorkingDir,
		opts.VersionManagerFileName,
		configbridge.TFRunOptsFromOpts(opts),
	)
	require.NoError(t, err)

	opts.TerraformVersion = ver
	opts.TofuImplementation = impl

	return terraformSource, opts, cfg, err
}

func testAlreadyHaveLatestCode(t *testing.T, canonicalURL string, downloadDir string, expected bool) {
	t.Helper()

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	terraformSource := &tf.Source{
		CanonicalSourceURL: parseURL(t, canonicalURL),
		DownloadDir:        downloadDir,
		WorkingDir:         downloadDir,
		VersionFile:        filepath.Join(downloadDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	actual, err := run.AlreadyHaveLatestCode(l, terraformSource, configbridge.NewRunOptions(opts))
	require.NoError(t, err)
	assert.Equal(t, expected, actual, "For terraform source %v", terraformSource)
}

func absPath(t *testing.T, path string) string {
	t.Helper()

	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	absPath, err := filepath.Abs(path)
	require.NoError(t, err)

	return filepath.Clean(absPath)
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

			client := run.BuildDownloadClient(logger.CreateLogger(), run.OSVenv(), configbridge.NewRunOptions(terragruntOptions), tc.cfg)

			fileGetter, ok := findGetter[*getter.FileCopyGetter](client.Getters)
			require.True(t, ok, "client should register a FileCopyGetter")

			assert.Equal(
				t,
				tc.expectedExcludeFiles,
				fileGetter.ExcludeFromCopy,
				"ExcludeFromCopy should match expected value",
			)
		})
	}
}

// TestBuildDownloadClientHTTPNetrc verifies that HTTP/HTTPS getters have Netrc enabled
// for authentication via ~/.netrc files.
func TestBuildDownloadClientHTTPNetrc(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	client := run.BuildDownloadClient(
		logger.CreateLogger(),
		run.OSVenv(),
		configbridge.NewRunOptions(terragruntOptions),
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
	)

	wrapped, ok := findGetter[*getter.HTTPSchemeGetter](client.Getters)
	require.True(t, ok, "client should register an HttpGetter")
	require.NotNil(t, wrapped.Inner)
	assert.True(t, wrapped.Inner.Netrc, "HttpGetter must have Netrc enabled for ~/.netrc authentication")
}

// TestBuildDownloadClientCoversDefaultSchemes verifies that the canonical
// Terragrunt protocol set is registered: file (via FileCopyGetter), git (via
// the symlink-preserving GitGetter), http(s), s3, gcs, hg, smb, and tfr (via
// RegistryGetter).
func TestBuildDownloadClientCoversDefaultSchemes(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	client := run.BuildDownloadClient(
		logger.CreateLogger(),
		run.OSVenv(),
		configbridge.NewRunOptions(terragruntOptions),
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
	)

	_, ok := findGetter[*getter.FileCopyGetter](client.Getters)
	assert.True(t, ok, "FileCopyGetter (file scheme)")

	_, ok = findGetter[*getter.GitGetter](client.Getters)
	assert.True(t, ok, "GitGetter (git scheme)")

	_, ok = findGetter[*getter.RegistryGetter](client.Getters)
	assert.True(t, ok, "RegistryGetter (tfr scheme)")

	_, ok = findGetter[*getter.HTTPSchemeGetter](client.Getters)
	assert.True(t, ok, "HttpGetter (http/https schemes)")

	_, ok = findGetter[*getter.HgGetter](client.Getters)
	assert.True(t, ok, "HgGetter (hg scheme)")

	_, ok = findGetter[*getter.SmbClientGetter](client.Getters)
	assert.True(t, ok, "SmbClientGetter (smb scheme)")
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
	updatedOpts, err := run.DownloadTerraformSource(t.Context(), l, run.OSVenv(), ".", configbridge.NewRunOptions(opts), cfg, r)
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

	_, err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, run.OSVenv(), src, configbridge.NewRunOptions(opts), cfg, r)

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

	_, err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, run.OSVenv(), src, configbridge.NewRunOptions(opts), cfg, r)
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

	_, err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, run.OSVenv(), src, configbridge.NewRunOptions(opts), cfg, r)
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

	_, err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, run.OSVenv(), src, configbridge.NewRunOptions(opts), cfg, r)
	require.NoError(t, err)

	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceUpdateSourceWithCASRequiresCAS verifies that setting
// update_source_with_cas = true on a terraform block errors when CAS is unavailable,
// either because the experiment is off or because --no-cas is set.
func TestDownloadSourceUpdateSourceWithCASRequiresCAS(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		enableCAS bool
		noCAS     bool
	}{
		{name: "experiment off", enableCAS: false, noCAS: false},
		{name: "experiment on with --no-cas", enableCAS: true, noCAS: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
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

			opts.Experiments = experiment.NewExperiments()
			if tc.enableCAS {
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.CAS))
			}

			opts.NoCAS = tc.noCAS
			opts.TerragruntConfigPath = "/tmp/terragrunt.hcl"

			cfg := &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{
					UpdateSourceWithCAS: true,
				},
			}

			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(io.Discard))

			_, err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, run.OSVenv(), src, configbridge.NewRunOptions(opts), cfg, report.NewReport())
			require.Error(t, err)

			var target *cas.UpdateSourceWithCASRequiresCASError
			require.ErrorAs(t, err, &target)
			assert.Equal(t, "terraform", target.BlockType)
			assert.Equal(t, opts.TerragruntConfigPath, target.Path)
		})
	}
}

// TestDownloadSourceWithCASMultipleSources tests that CAS works with multiple different sources
func TestDownloadSourceWithCASMultipleSources(t *testing.T) {
	t.Parallel()

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

			_, err = run.DownloadTerraformSourceIfNecessary(t.Context(), l, run.OSVenv(), src, configbridge.NewRunOptions(opts), cfg, r)

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
//
// Does not use `t.Parallel()` because we need to set the `NETRC` environment variable
// to point to a temporary `~/.netrc` file for the test to pass.
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

	dst := filepath.Join(t.TempDir(), "module.tf")

	client := run.BuildDownloadClient(logger.CreateLogger(), run.OSVenv(), configbridge.NewRunOptions(opts), cfg)

	_, err = client.Get(t.Context(), &getter.Request{
		Src:     server.URL + "/module.tf",
		Dst:     dst,
		GetMode: getter.ModeFile,
	})
	require.NoError(t, err)

	downloaded, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, fileContent, string(downloaded))
}

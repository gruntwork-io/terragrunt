package run_test

import (
	"bytes"
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
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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
	downloadDir := absPath(t, "does-not-exist")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirDoesNotExist(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com"
	downloadDir := absPath(t, "does-not-exist")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsNoVersionNoVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com"
	downloadDir := absPath(t, "../../../test/fixtures/download-source/download-dir-empty")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsNoVersionWithVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com"
	downloadDir := absPath(t, "../../../test/fixtures/download-source/download-dir-version-file-no-query")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, true)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionNoVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := absPath(t, "../../../test/fixtures/download-source/download-dir-empty")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionAndVersionFile(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := absPath(t, "../../../test/fixtures/download-source/download-dir-version-file")

	testAlreadyHaveLatestCode(t, canonicalURL, downloadDir, false)
}

func TestAlreadyHaveLatestCodeRemoteFilePathDownloadDirExistsWithVersionAndVersionFileAndTfCode(t *testing.T) {
	t.Parallel()

	canonicalURL := "http://www.some-url.com?ref=v0.0.1"
	downloadDir := absPath(t, "../../../test/fixtures/download-source/download-dir-version-file-tf-code")

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

	srv := helpers.NewGitServer(t)
	srv.AddFixtures("test/fixtures/download-source/hello-world")
	canonicalURL := srv.SourceURL("test/fixtures/download-source/hello-world", "")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDir(t *testing.T) {
	t.Parallel()

	srv := helpers.NewGitServer(t)
	srv.AddFixtures("test/fixtures/download-source/hello-world")
	canonicalURL := srv.SourceURL("test/fixtures/download-source/hello-world", "")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World 2", false)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirDifferentVersion(t *testing.T) {
	t.Parallel()

	srv := helpers.NewGitServer(t)
	srv.AddFixtures("test/fixtures/download-source/hello-world")
	canonicalURL := srv.SourceURL("test/fixtures/download-source/hello-world", "v0.83.2")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-2", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, false, "# Hello, World", true)
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlToAlreadyDownloadedDirSameVersion(t *testing.T) {
	t.Parallel()

	srv := helpers.NewGitServer(t)
	srv.AddFixtures("test/fixtures/download-source/hello-world-version-remote")
	canonicalURL := srv.SourceURL("test/fixtures/download-source/hello-world-version-remote", "v0.83.2")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, opts, cfg, err := createConfig(t, canonicalURL, downloadDir, false)
	require.NoError(t, err)

	// The hello-world-version-remote fixture ships a file literally named
	// "version-file.txt". CAS materializes downloaded sources read-only, so a
	// bookkeeping write to that path would collide with the module's own file.
	// Use the name Terragrunt actually writes in production
	// (.terragrunt-source-version), which never appears in module content.
	terraformSource.VersionFile = filepath.Join(downloadDir, ".terragrunt-source-version")

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		venv.OSVenv(),
		terraformSource,
		configbridge.NewRunOptions(opts),
		cfg,
		report.NewReport(),
	)
	require.NoError(t, err, "For terraform source %v: %v", terraformSource, err)

	expectedFilePath := filepath.Join(downloadDir, "main.tf")
	if assert.True(t, util.FileExists(expectedFilePath), "For terraform source %v", terraformSource) {
		actualFileContents := readFile(t, expectedFilePath)
		assert.Equal(t, "# Hello, World version remote", actualFileContents, "For terraform source %v", terraformSource)
	}
}

func TestDownloadTerraformSourceIfNecessaryRemoteUrlOverrideSource(t *testing.T) {
	t.Parallel()

	srv := helpers.NewGitServer(t)
	srv.AddFixtures("test/fixtures/download-source/hello-world")
	canonicalURL := srv.SourceURL("test/fixtures/download-source/hello-world", "v0.83.2")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	testDownloadTerraformSourceIfNecessary(t, canonicalURL, downloadDir, true, "# Hello, World", false)
}

func TestDownloadTerraformSourceIfNecessaryInvalidTerraformSource(t *testing.T) {
	t.Parallel()

	// v1.2.3 is not among the server's seeded tags, so the clone fails
	// offline and the download is reported as a DownloadingTerraformSourceErr.
	srv := helpers.NewGitServer(t)
	canonicalURL := srv.SourceURL("test/fixtures/download-source/hello-world", "v1.2.3")

	downloadDir := helpers.TmpDirWOSymlinks(t)
	defer os.Remove(downloadDir)

	copyFolder(t, "../../../test/fixtures/download-source/hello-world-version-remote", downloadDir)

	terraformSource, opts, cfg, err := createConfig(t, canonicalURL, downloadDir, false)

	require.NoError(t, err)

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		venv.OSVenv(),
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

	canonicalURL := "github.com/gruntwork-io/terragrunt//test/fixtures/" +
		"download-source/hello-world-version-remote/non-existent-path?ref=v0.83.2"

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
		venv.OSVenv(),
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
	// download_source test depend on tofu being installed. Any invocation
	// other than the version probe is a regression: fail loudly rather
	// than silently absorb it.
	versionExec := vexec.NewMemExec(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		// DefaultWrappedPath resolves to either tofu or terraform depending
		// on what's on the host PATH; accept both so the assertion stays
		// host-independent.
		if (inv.Name != "tofu" && inv.Name != "terraform") || !slices.Contains(inv.Args, "-version") {
			assert.Fail(t, "unexpected invocation during PopulateTFVersion",
				"name=%q args=%v", inv.Name, inv.Args)

			return vexec.Result{ExitCode: 1}
		}

		return vexec.Result{Stdout: []byte("OpenTofu v1.7.2\n")}
	})

	versionV := venvtest.New().WithExec(versionExec).WithEnv(venv.OSVenv().Env)
	_, ver, impl, err := run.PopulateTFVersion(t.Context(), l, versionV, run.PopulateTFVersionInput{
		TFOpts:       configbridge.TFRunOptsFromOpts(opts),
		WorkingDir:   opts.WorkingDir,
		VersionFiles: opts.VersionManagerFileName,
	})
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
		absPath(t, filepath.FromSlash(src)),
		absPath(t, filepath.FromSlash(dest)),
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

			client, err := run.BuildDownloadClient(
				logger.CreateLogger(),
				venv.OSVenv(),
				configbridge.NewRunOptions(terragruntOptions),
				tc.cfg,
			)
			require.NoError(t, err)

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

	client, err := run.BuildDownloadClient(
		logger.CreateLogger(),
		venv.OSVenv(),
		configbridge.NewRunOptions(terragruntOptions),
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
	)
	require.NoError(t, err)

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

	client, err := run.BuildDownloadClient(
		logger.CreateLogger(),
		venv.OSVenv(),
		configbridge.NewRunOptions(terragruntOptions),
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
	)
	require.NoError(t, err)

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
	updatedOpts, err := run.DownloadTerraformSource(
		t.Context(),
		l,
		venv.OSVenv(),
		".",
		configbridge.NewRunOptions(opts),
		cfg,
		r,
	)
	require.NoError(t, err)

	// Verify that the working directory was changed to the cache directory (inside downloadDir)
	assert.NotEqual(t, sourceDir, updatedOpts.CacheDir, "Working dir should be changed to cache")
	assert.True(t, strings.HasPrefix(updatedOpts.CacheDir, downloadDir), "Working dir should be under download dir")

	// Verify that the main.tf file was copied to the cache
	cachedMainTf := filepath.Join(updatedOpts.CacheDir, "main.tf")
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

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		l,
		venv.OSVenv(),
		src,
		configbridge.NewRunOptions(opts),
		cfg,
		r,
	)

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

	opts.Experiments = experiment.NewExperiments()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		l,
		venv.OSVenv(),
		src,
		configbridge.NewRunOptions(opts),
		cfg,
		r,
	)
	require.NoError(t, err)

	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceOCIThroughCASExperimentGate: with the oci experiment on,
// an oci source enters the CAS path (observed via the CAS attempt log) and
// reaches the real getter; off, the CAS attempt is skipped up front.
func TestDownloadSourceOCIThroughCASExperimentGate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		enableOCI bool
	}{
		{name: "experiment enabled reaches the oci getter", enableOCI: true},
		{name: "experiment disabled skips the oci getter", enableOCI: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			// A TLS server the test owns: the client rejects its self-signed
			// cert deterministically, with no assumptions about closed ports.
			registry := httptest.NewTLSServer(http.NotFoundHandler())
			t.Cleanup(registry.Close)

			registryAddr := registry.Listener.Addr().String()
			src := &tf.Source{
				CanonicalSourceURL: parseURL(t, "oci://"+registryAddr+"/terraform-modules/vpc?tag=1.0.0"),
				DownloadDir:        tmpDir,
				WorkingDir:         tmpDir,
				VersionFile:        filepath.Join(tmpDir, "version-file.txt"),
			}

			opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
			require.NoError(t, err)

			opts.Experiments = experiment.NewExperiments()

			if tc.enableOCI {
				require.NoError(t, opts.Experiments.EnableExperiment(experiment.OCI))
			}

			cfg := &runcfg.RunConfig{
				Terraform: runcfg.TerraformConfig{
					ExtraArgs: []runcfg.TerraformExtraArguments{},
				},
			}

			var logBuf bytes.Buffer

			l := logger.CreateLogger()
			l.SetOptions(log.WithOutput(&logBuf), log.WithLevel(log.DebugLevel))

			_, err = run.DownloadTerraformSourceIfNecessary(
				t.Context(),
				l,
				venv.OSVenv(),
				src,
				configbridge.NewRunOptions(opts),
				cfg,
				report.NewReport(),
			)
			require.Error(t, err, "the fake registry's cert is untrusted, so every fetch fails")

			const casAttempt = "CAS enabled: attempting to use Content Addressable Storage"

			if tc.enableOCI {
				require.ErrorContains(t, err, "resolving OCI reference", "the oci getter must run when the experiment is on")
				assert.Contains(t, logBuf.String(), casAttempt, "the oci source must enter the CAS path when the experiment is on")

				return
			}

			assert.NotContains(t, err.Error(), "resolving OCI reference", "no oci getter must run when the experiment is off")
			assert.NotContains(t, logBuf.String(), casAttempt, "the CAS attempt must be skipped when the experiment is off")
		})
	}
}

// TestDownloadSourceWithCASGitSource tests CAS functionality with a Git source
func TestDownloadSourceWithCASGitSource(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	srv := helpers.NewGitServer(t)
	srv.AddFixtures("test/fixtures/download/hello-world")

	src := &tf.Source{
		CanonicalSourceURL: parseURL(t, srv.SourceURL("test/fixtures/download/hello-world", "")),
		DownloadDir:        tmpDir,
		WorkingDir:         tmpDir,
		VersionFile:        filepath.Join(tmpDir, "version-file.txt"),
	}

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	opts.Experiments = experiment.NewExperiments()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		l,
		venv.OSVenv(),
		src,
		configbridge.NewRunOptions(opts),
		cfg,
		r,
	)
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

	opts.Experiments = experiment.NewExperiments()

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			ExtraArgs: []runcfg.TerraformExtraArguments{},
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	r := report.NewReport()

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		l,
		venv.OSVenv(),
		src,
		configbridge.NewRunOptions(opts),
		cfg,
		r,
	)
	require.NoError(t, err)

	expectedFilePath := filepath.Join(tmpDir, "main.tf")
	assert.FileExists(t, expectedFilePath)
}

// TestDownloadSourceUpdateSourceWithCASRequiresCAS verifies that setting
// update_source_with_cas = true on a terraform block errors when CAS is
// disabled via --no-cas.
func TestDownloadSourceUpdateSourceWithCASRequiresCAS(t *testing.T) {
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

	opts.NoCAS = true
	opts.TerragruntConfigPath = "/tmp/terragrunt.hcl"

	cfg := &runcfg.RunConfig{
		Terraform: runcfg.TerraformConfig{
			UpdateSourceWithCAS: true,
		},
	}

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(), l, venv.OSVenv(), src,
		configbridge.NewRunOptions(opts),
		cfg, report.NewReport(),
	)
	require.Error(t, err)

	var target *cas.UpdateSourceWithCASRequiresCASError
	require.ErrorAs(t, err, &target)
	assert.Equal(t, "terraform", target.BlockType)
	assert.Equal(t, opts.TerragruntConfigPath, target.Path)
}

// TestDownloadSourceWithCASMultipleSources tests that CAS works with multiple different sources
func TestDownloadSourceWithCASMultipleSources(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("./should-not-be-used")
	require.NoError(t, err)

	// Enable CAS experiment
	opts.Experiments = experiment.NewExperiments()

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

			_, err = run.DownloadTerraformSourceIfNecessary(
				t.Context(),
				l,
				venv.OSVenv(),
				src,
				configbridge.NewRunOptions(opts),
				cfg,
				r,
			)

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

	client, err := run.BuildDownloadClient(logger.CreateLogger(), venv.OSVenv(), configbridge.NewRunOptions(opts), cfg)
	require.NoError(t, err)

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

// TestDownloadTerraformSourceRejectsNonOSFilesystem pins that the entry
// guard returns ErrNonOSFilesystem before any download work runs when
// Options.FS is not OS-backed.
func TestDownloadTerraformSourceRejectsNonOSFilesystem(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	runOpts := configbridge.NewRunOptions(opts)
	runOpts.FS = vfs.NewMemMapFS()

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	_, err = run.DownloadTerraformSource(
		t.Context(),
		l,
		venv.OSVenv(),
		".",
		runOpts,
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
		report.NewReport(),
	)
	require.ErrorIs(t, err, run.ErrNonOSFilesystem)
}

// TestDownloadTerraformSourceIfNecessaryRejectsNonOSFilesystem pins the guard
// on the exported helper so external callers cannot bypass the OS-FS invariant.
func TestDownloadTerraformSourceIfNecessaryRejectsNonOSFilesystem(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	runOpts := configbridge.NewRunOptions(opts)
	runOpts.FS = vfs.NewMemMapFS()

	src, err := tf.NewSource(logger.CreateLogger(), ".", t.TempDir(), opts.WorkingDir, false)
	require.NoError(t, err)

	_, err = run.DownloadTerraformSourceIfNecessary(
		t.Context(),
		logger.CreateLogger(),
		venv.OSVenv(),
		src,
		runOpts,
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
		report.NewReport(),
	)
	require.ErrorIs(t, err, run.ErrNonOSFilesystem)
}

// TestBuildDownloadClientRejectsNonOSFilesystem pins the guard on the
// exported client constructor so callers cannot construct a client that would
// later hand a non-OS FS to FileCopyGetter or RegistryGetter.
func TestBuildDownloadClientRejectsNonOSFilesystem(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("./test")
	require.NoError(t, err)

	runOpts := configbridge.NewRunOptions(opts)

	v := venv.OSVenv()
	v.FS = vfs.NewMemMapFS()

	client, err := run.BuildDownloadClient(
		logger.CreateLogger(),
		v,
		runOpts,
		&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
	)
	require.ErrorIs(t, err, run.ErrNonOSFilesystem)
	assert.Nil(t, client)
}

// TestBuildDownloadClientOCIExperimentGate verifies that the oci getter is
// registered only when the oci experiment is enabled: without it, oci://
// sources keep failing with the generic go-getter error; with it, the typed
// OCI validation error proves the getter runs.
func TestBuildDownloadClientOCIExperimentGate(t *testing.T) {
	t.Parallel()

	if helpers.IsExperimentMode(t) {
		t.Skip("Skipping the disabled-vs-enabled comparison in experiment mode")
	}

	testCases := []struct {
		name    string
		enabled bool
	}{
		{
			name:    "experiment disabled keeps oci unregistered",
			enabled: false,
		},
		{
			name:    "experiment enabled registers the oci getter",
			enabled: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			terragruntOptions, err := options.NewTerragruntOptionsForTest("./test")
			require.NoError(t, err)

			if tc.enabled {
				require.NoError(t, terragruntOptions.Experiments.EnableExperiment(experiment.OCI))
			}

			client, err := run.BuildDownloadClient(
				logger.CreateLogger(),
				venv.OSVenv(),
				configbridge.NewRunOptions(terragruntOptions),
				&runcfg.RunConfig{Terraform: runcfg.TerraformConfig{}},
			)
			require.NoError(t, err)

			_, found := findGetter[*getter.OCIGetter](client.Getters)
			assert.Equal(t, tc.enabled, found)

			dst := filepath.Join(t.TempDir(), "module")

			_, err = client.Get(t.Context(), &getter.Request{
				Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?bogus=1",
				Dst:     dst,
				GetMode: getter.ModeDir,
			})
			require.Error(t, err)
			assert.Equal(t, tc.enabled, errors.Is(err, getter.OCIUnsupportedQueryParamError{Param: "bogus"}))
		})
	}
}

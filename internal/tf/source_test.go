package tf_test

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/strict"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// fileSourceURL builds a file:// CanonicalSourceURL pointing at dir, matching what
// tf.NewSource produces for a local source.
func fileSourceURL(t *testing.T, dir string) *url.URL {
	t.Helper()

	u, err := url.Parse("file://" + filepath.ToSlash(dir))
	require.NoError(t, err)

	return u
}

func TestSplitSourceUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		sourceURL          string
		expectedSo         string
		expectedModulePath string
	}{
		{"root-path-only-no-double-slash", "/foo", "/foo", ""},
		{"parent-path-one-child-no-double-slash", "/foo/bar", "/foo/bar", ""},
		{"parent-path-multiple-children-no-double-slash", "/foo/bar/baz/blah", "/foo/bar/baz/blah", ""},
		{"relative-path-no-children-no-double-slash", "../foo", "../foo", ""},
		{"relative-path-one-child-no-double-slash", "../foo/bar", "../foo/bar", ""},
		{"relative-path-multiple-children-no-double-slash", "../foo/bar/baz/blah", "../foo/bar/baz/blah", ""},
		{"root-path-only-with-double-slash", "/foo//", "/foo", ""},
		{"parent-path-one-child-with-double-slash", "/foo//bar", "/foo", "bar"},
		{"parent-path-multiple-children-with-double-slash", "/foo/bar//baz/blah", "/foo/bar", "baz/blah"},
		{"relative-path-no-children-with-double-slash", "..//foo", "..", "foo"},
		{"relative-path-one-child-with-double-slash", "../foo//bar", "../foo", "bar"},
		{"relative-path-multiple-children-with-double-slash", "../foo/bar//baz/blah", "../foo/bar", "baz/blah"},
		{"parent-url-one-child-no-double-slash", "ssh://git@github.com/foo/modules.git/foo", "ssh://git@github.com/foo/modules.git/foo", ""},
		{"parent-url-multiple-children-no-double-slash", "ssh://git@github.com/foo/modules.git/foo/bar/baz/blah", "ssh://git@github.com/foo/modules.git/foo/bar/baz/blah", ""},
		{"parent-url-one-child-with-double-slash", "ssh://git@github.com/foo/modules.git//foo", "ssh://git@github.com/foo/modules.git", "foo"},
		{"parent-url-multiple-children-with-double-slash", "ssh://git@github.com/foo/modules.git//foo/bar/baz/blah", "ssh://git@github.com/foo/modules.git", "foo/bar/baz/blah"},
		{"separate-ref-with-slash", "ssh://git@github.com/foo/modules.git//foo?ref=feature/modules", "ssh://git@github.com/foo/modules.git?ref=feature/modules", "foo"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sourceURL, err := url.Parse(tc.sourceURL)
			require.NoError(t, err)

			l := logger.CreateLogger()

			actualRootRepo, actualModulePath, err := tf.SplitSourceURL(l, sourceURL)
			require.NoError(t, err)

			assert.Equal(t, tc.expectedSo, actualRootRepo.String())
			assert.Equal(t, tc.expectedModulePath, actualModulePath)
		})
	}
}

func TestToSourceUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		sourceURL         string
		expectedSourceURL string
	}{
		{"https://github.com/gruntwork-io/repo-name", "git::https://github.com/gruntwork-io/repo-name.git"},
		{"git::https://github.com/gruntwork-io/repo-name", "git::https://github.com/gruntwork-io/repo-name"},
		{"https://github.com/gruntwork-io/repo-name//modules/module-name", "git::https://github.com/gruntwork-io/repo-name.git//modules/module-name"},
		{"ssh://github.com/gruntwork-io/repo-name//modules/module-name", "ssh://github.com/gruntwork-io/repo-name//modules/module-name"},
		{"https://gitlab.com/catamphetamine/libphonenumber-js", "git::https://gitlab.com/catamphetamine/libphonenumber-js.git"},
		{"https://bitbucket.org/atlassian/aws-ecr-push-image", "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git"},
		{"http://bitbucket.org/atlassian/aws-ecr-push-image", "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git"},
		{"https://s3-eu-west-1.amazonaws.com/modules/vpc.zip", "https://s3-eu-west-1.amazonaws.com/modules/vpc.zip"},
		// Public GCS URLs route through the http(s) getter (no GCP creds), matching
		// how public S3 URLs are handled. To force the credentialed gcs getter
		// callers must use the explicit `gcs::` forced-getter prefix.
		{"https://www.googleapis.com/storage/v1/modules/foomodule.zip", "https://www.googleapis.com/storage/v1/modules/foomodule.zip"},
		{"gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip", "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip"},
		{"git::https://name@dev.azure.com/name/project-name/_git/repo-name", "git::https://name@dev.azure.com/name/project-name/_git/repo-name"},
		{"https://repository.rnd.net/artifactory/generic-production-iac/tf-auto-azr-iam.2.6.0.zip", "https://repository.rnd.net/artifactory/generic-production-iac/tf-auto-azr-iam.2.6.0.zip"},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actualSourceURL, err := tf.ToSourceURL(tc.sourceURL, os.TempDir())
			require.NoError(t, err)
			assert.Equal(t, tc.expectedSourceURL, actualSourceURL.String())
		})
	}
}

// TestRewriteLegacyGCSPublicSource pins the rewrite, the strict-control
// disable, and the no-op cases. Each row constructs its own control instance
// so the per-control sync.Once dedup starts fresh and warnings don't bleed
// between sub-tests.
func TestRewriteLegacyGCSPublicSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		source         string
		want           string
		controlEnabled bool
	}{
		{
			name:   "public storage URL is rewritten",
			source: "https://www.googleapis.com/storage/v1/modules/foo.zip",
			want:   "gcs::https://www.googleapis.com/storage/v1/modules/foo.zip",
		},
		{
			name:   "http storage URL is rewritten",
			source: "http://www.googleapis.com/storage/v1/modules/foo.zip",
			want:   "gcs::http://www.googleapis.com/storage/v1/modules/foo.zip",
		},
		{
			name:   "explicit gcs:: prefix passes through",
			source: "gcs::https://www.googleapis.com/storage/v1/modules/foo.zip",
			want:   "gcs::https://www.googleapis.com/storage/v1/modules/foo.zip",
		},
		{
			name:   "non-storage googleapis path passes through",
			source: "https://www.googleapis.com/auth/v1/token",
			want:   "https://www.googleapis.com/auth/v1/token",
		},
		{
			name:   "unrelated host passes through",
			source: "https://example.com/foo.zip",
			want:   "https://example.com/foo.zip",
		},
		{
			name:           "control enabled disables the rewrite",
			source:         "https://www.googleapis.com/storage/v1/modules/foo.zip",
			controlEnabled: true,
			want:           "https://www.googleapis.com/storage/v1/modules/foo.zip",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			ctrls := strict.Controls{
				&controls.Control{
					Name:    controls.LegacyGCSPublicPrefix,
					Warning: controls.LegacyGCSDeprecationWarning,
					Enabled: tc.controlEnabled,
				},
			}

			got := tf.RewriteLegacyGCSPublicSource(t.Context(), l, tc.source, ctrls)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestRewriteLegacyGCSPublicSourceMissingControl pins the fallback when ctrls
// has no LegacyGCSPublicPrefix entry: rewrite applied, no warning.
func TestRewriteLegacyGCSPublicSourceMissingControl(t *testing.T) {
	t.Parallel()

	got := tf.RewriteLegacyGCSPublicSource(
		t.Context(),
		logger.CreateLogger(),
		"https://www.googleapis.com/storage/v1/modules/foo.zip",
		nil,
	)
	assert.Equal(t, "gcs::https://www.googleapis.com/storage/v1/modules/foo.zip", got)
}

func TestRegressionSupportForGitRemoteCodecommit(t *testing.T) {
	t.Parallel()

	source := "git::codecommit::ap-northeast-1://my_app_modules//my-app/modules/main-module"
	sourceURL, err := tf.ToSourceURL(source, ".")
	require.NoError(t, err)
	require.Equal(t, "git::codecommit::ap-northeast-1", sourceURL.Scheme)

	l := logger.CreateLogger()

	actualRootRepo, actualModulePath, err := tf.SplitSourceURL(l, sourceURL)
	require.NoError(t, err)

	require.Equal(t, "git::codecommit::ap-northeast-1://my_app_modules", actualRootRepo.String())
	require.Equal(t, "my-app/modules/main-module", actualModulePath)
}

// TestRegressionCASRefPreservesSubdir checks that SplitSourceURL splits the
// "//" subdir out of a cas:: reference, whose subdir lives in the URL's opaque
// component rather than its path.
func TestRegressionCASRefPreservesSubdir(t *testing.T) {
	t.Parallel()

	const hash = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

	sourceURL, err := tf.ToSourceURL("cas::sha1:"+hash+"//modules/vpc", ".")
	require.NoError(t, err)
	require.Equal(t, "cas::sha1", sourceURL.Scheme)

	l := logger.CreateLogger()

	actualRootRepo, actualModulePath, err := tf.SplitSourceURL(l, sourceURL)
	require.NoError(t, err)

	require.Equal(t, "cas::sha1:"+hash, actualRootRepo.String())
	require.Equal(t, "modules/vpc", actualModulePath)
}

// TestRegressionCASRefSubdirWorkingDir checks that NewSource points the working
// directory at the subdir of a cas:: reference while keeping the canonical
// source free of it, so the getter downloads the whole tree.
func TestRegressionCASRefSubdirWorkingDir(t *testing.T) {
	t.Parallel()

	const hash = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

	l := logger.CreateLogger()
	downloadDir := t.TempDir()

	src, err := tf.NewSource(l, "cas::sha1:"+hash+"//modules/vpc", downloadDir, t.TempDir(), false)
	require.NoError(t, err)

	assert.Equal(t, "cas::sha1:"+hash, src.CanonicalSourceURL.String())
	assert.Equal(t, filepath.Join(src.DownloadDir, "modules", "vpc"), src.WorkingDir)
}

// TestRegressionCASRefNoSubdir checks that a cas:: reference without a "//"
// subdir yields an empty module path: SplitSourceURL preserves the whole ref as
// the root repo, and NewSource leaves the working directory at the download
// directory.
func TestRegressionCASRefNoSubdir(t *testing.T) {
	t.Parallel()

	const hash = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

	sourceURL, err := tf.ToSourceURL("cas::sha1:"+hash, ".")
	require.NoError(t, err)
	require.Equal(t, "cas::sha1", sourceURL.Scheme)

	l := logger.CreateLogger()

	actualRootRepo, actualModulePath, err := tf.SplitSourceURL(l, sourceURL)
	require.NoError(t, err)

	require.Equal(t, "cas::sha1:"+hash, actualRootRepo.String())
	require.Empty(t, actualModulePath)

	downloadDir := t.TempDir()

	src, err := tf.NewSource(l, "cas::sha1:"+hash, downloadDir, t.TempDir(), false)
	require.NoError(t, err)

	assert.Equal(t, "cas::sha1:"+hash, src.CanonicalSourceURL.String())
	assert.Equal(t, src.DownloadDir, src.WorkingDir)
}

// TestEncodeSourceVersionIgnoresFilesNeverCopied is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/6443: EncodeSourceVersion hashed every file
// under the source directory, including hidden files that util.CopyFolderContents (via
// util.TerragruntExcludes) never copies into the cache. A file that can never reach the cache
// must not be able to change the version hash.
func TestEncodeSourceVersionIgnoresFilesNeverCopied(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# main"), 0o644))

	src := &tf.Source{CanonicalSourceURL: fileSourceURL(t, sourceDir)}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	// Mirrors `touch .this_file_does_not_matter` from the bug report.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, ".this_file_does_not_matter"), []byte("noise"), 0o644))

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.Equal(t, before, after, "creating a hidden file that is never copied must not change the source version hash")
}

// TestEncodeSourceVersionStillIgnoresLockFile pins the pre-existing behavior that the
// terraform/tofu lock file never contributes to the hash, since its content is auto-generated.
func TestEncodeSourceVersionStillIgnoresLockFile(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# main"), 0o644))

	src := &tf.Source{CanonicalSourceURL: fileSourceURL(t, sourceDir)}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, util.TerraformLockFile), []byte("# lock"), 0o644))

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.Equal(t, before, after, "the lock file must never affect the hash")
}

// TestEncodeSourceVersionStillSkipsIgnorableDirs pins the pre-existing behavior that .git,
// .terraform, and .terragrunt-cache are never descended into, regardless of the new copy-filter
// check added alongside them.
func TestEncodeSourceVersionStillSkipsIgnorableDirs(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# main"), 0o644))

	src := &tf.Source{CanonicalSourceURL: fileSourceURL(t, sourceDir)}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	for _, dir := range []string{util.GitDir, util.TerraformCacheDir, util.TerragruntCacheDir} {
		dirPath := filepath.Join(sourceDir, dir)
		require.NoError(t, os.MkdirAll(dirPath, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dirPath, "noise.txt"), []byte("noise"), 0o644))
	}

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.Equal(t, before, after, "files under .git, .terraform, and .terragrunt-cache must never affect the hash")
}

// TestEncodeSourceVersionDetectsVisibleFileChange is a no-regression baseline: a tracked,
// non-hidden file's mtime change must still change the hash so a real init still runs.
func TestEncodeSourceVersionDetectsVisibleFileChange(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	mainTfPath := filepath.Join(sourceDir, "main.tf")
	require.NoError(t, os.WriteFile(mainTfPath, []byte("# main"), 0o644))

	src := &tf.Source{CanonicalSourceURL: fileSourceURL(t, sourceDir)}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	future := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(mainTfPath, future, future))

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "a visible tracked file's mtime change must still change the hash")
}

// TestEncodeSourceVersionRespectsIncludeInCopy checks that a hidden file matched by
// IncludeInCopy still affects the hash, since util.CopyFolderContents will copy it into the
// cache. Source.IncludeInCopy mirrors the terraform block's include_in_copy setting.
func TestEncodeSourceVersionRespectsIncludeInCopy(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# main"), 0o644))

	src := &tf.Source{
		CanonicalSourceURL: fileSourceURL(t, sourceDir),
		IncludeInCopy:      []string{".tflint.hcl"},
	}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, ".tflint.hcl"), []byte("config {}"), 0o644))

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.NotEqual(t, before, after, "a file matched by IncludeInCopy must still affect the hash")
}

// TestEncodeSourceVersionRespectsExcludeFromCopy checks that a visible file matched by
// ExcludeFromCopy is excluded from the hash, since util.CopyFolderContents never copies it.
// Source.ExcludeFromCopy mirrors the terraform block's exclude_from_copy setting.
func TestEncodeSourceVersionRespectsExcludeFromCopy(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# main"), 0o644))

	src := &tf.Source{
		CanonicalSourceURL: fileSourceURL(t, sourceDir),
		ExcludeFromCopy:    []string{"*.bak"},
	}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "notes.bak"), []byte("scratch"), 0o644))

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.Equal(t, before, after, "a file matched by ExcludeFromCopy must not affect the hash")
}

// TestEncodeSourceVersionExcludeFromCopyDirectory checks that an entire directory matched by
// ExcludeFromCopy is skipped, so files added underneath it never affect the hash.
func TestEncodeSourceVersionExcludeFromCopyDirectory(t *testing.T) {
	t.Parallel()

	sourceDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.tf"), []byte("# main"), 0o644))

	src := &tf.Source{
		CanonicalSourceURL: fileSourceURL(t, sourceDir),
		ExcludeFromCopy:    []string{"scratch"},
	}
	l := logger.CreateLogger()

	before, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	scratchDir := filepath.Join(sourceDir, "scratch")
	require.NoError(t, os.MkdirAll(scratchDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(scratchDir, "file.txt"), []byte("noise"), 0o644))

	after, err := src.EncodeSourceVersion(l)
	require.NoError(t, err)

	assert.Equal(t, before, after, "files under a directory matched by ExcludeFromCopy must not affect the hash")
}

package tf_test

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
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

func TestSplitSourceUrl(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		sourceURL          string
		expectedSo         string
		expectedModulePath string
	}{
		{
			name:       "root-path-only-no-double-slash",
			sourceURL:  "/foo",
			expectedSo: "/foo",
		},
		{
			name:       "parent-path-one-child-no-double-slash",
			sourceURL:  "/foo/bar",
			expectedSo: "/foo/bar",
		},
		{
			name:       "parent-path-multiple-children-no-double-slash",
			sourceURL:  "/foo/bar/baz/blah",
			expectedSo: "/foo/bar/baz/blah",
		},
		{
			name:       "relative-path-no-children-no-double-slash",
			sourceURL:  "../foo",
			expectedSo: "../foo",
		},
		{
			name:       "relative-path-one-child-no-double-slash",
			sourceURL:  "../foo/bar",
			expectedSo: "../foo/bar",
		},
		{
			name:       "relative-path-multiple-children-no-double-slash",
			sourceURL:  "../foo/bar/baz/blah",
			expectedSo: "../foo/bar/baz/blah",
		},
		{
			name:       "root-path-only-with-double-slash",
			sourceURL:  "/foo//",
			expectedSo: "/foo",
		},
		{
			name:               "parent-path-one-child-with-double-slash",
			sourceURL:          "/foo//bar",
			expectedSo:         "/foo",
			expectedModulePath: "bar",
		},
		{
			name:               "parent-path-multiple-children-with-double-slash",
			sourceURL:          "/foo/bar//baz/blah",
			expectedSo:         "/foo/bar",
			expectedModulePath: "baz/blah",
		},
		{
			name:               "relative-path-no-children-with-double-slash",
			sourceURL:          "..//foo",
			expectedSo:         "..",
			expectedModulePath: "foo",
		},
		{
			name:               "relative-path-one-child-with-double-slash",
			sourceURL:          "../foo//bar",
			expectedSo:         "../foo",
			expectedModulePath: "bar",
		},
		{
			name:               "relative-path-multiple-children-with-double-slash",
			sourceURL:          "../foo/bar//baz/blah",
			expectedSo:         "../foo/bar",
			expectedModulePath: "baz/blah",
		},
		{
			name:       "parent-url-one-child-no-double-slash",
			sourceURL:  "ssh://git@github.com/foo/modules.git/foo",
			expectedSo: "ssh://git@github.com/foo/modules.git/foo",
		},
		{
			name:       "parent-url-multiple-children-no-double-slash",
			sourceURL:  "ssh://git@github.com/foo/modules.git/foo/bar/baz/blah",
			expectedSo: "ssh://git@github.com/foo/modules.git/foo/bar/baz/blah",
		},
		{
			name:               "parent-url-one-child-with-double-slash",
			sourceURL:          "ssh://git@github.com/foo/modules.git//foo",
			expectedSo:         "ssh://git@github.com/foo/modules.git",
			expectedModulePath: "foo",
		},
		{
			name:               "parent-url-multiple-children-with-double-slash",
			sourceURL:          "ssh://git@github.com/foo/modules.git//foo/bar/baz/blah",
			expectedSo:         "ssh://git@github.com/foo/modules.git",
			expectedModulePath: "foo/bar/baz/blah",
		},
		{
			name:               "separate-ref-with-slash",
			sourceURL:          "ssh://git@github.com/foo/modules.git//foo?ref=feature/modules",
			expectedSo:         "ssh://git@github.com/foo/modules.git?ref=feature/modules",
			expectedModulePath: "foo",
		},
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
		{
			sourceURL:         "https://github.com/gruntwork-io/repo-name",
			expectedSourceURL: "git::https://github.com/gruntwork-io/repo-name.git",
		},
		{
			sourceURL:         "git::https://github.com/gruntwork-io/repo-name",
			expectedSourceURL: "git::https://github.com/gruntwork-io/repo-name",
		},
		{
			sourceURL:         "https://github.com/gruntwork-io/repo-name//modules/module-name",
			expectedSourceURL: "git::https://github.com/gruntwork-io/repo-name.git//modules/module-name",
		},
		{
			sourceURL:         "ssh://github.com/gruntwork-io/repo-name//modules/module-name",
			expectedSourceURL: "ssh://github.com/gruntwork-io/repo-name//modules/module-name",
		},
		{
			sourceURL:         "https://gitlab.com/catamphetamine/libphonenumber-js",
			expectedSourceURL: "git::https://gitlab.com/catamphetamine/libphonenumber-js.git",
		},
		{
			sourceURL:         "https://bitbucket.org/atlassian/aws-ecr-push-image",
			expectedSourceURL: "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git",
		},
		{
			sourceURL:         "http://bitbucket.org/atlassian/aws-ecr-push-image",
			expectedSourceURL: "git::https://bitbucket.org/atlassian/aws-ecr-push-image.git",
		},
		{
			sourceURL:         "https://s3-eu-west-1.amazonaws.com/modules/vpc.zip",
			expectedSourceURL: "https://s3-eu-west-1.amazonaws.com/modules/vpc.zip",
		},
		// Public GCS URLs route through the http(s) getter (no GCP creds), matching
		// how public S3 URLs are handled. To force the credentialed gcs getter
		// callers must use the explicit `gcs::` forced-getter prefix.
		{
			sourceURL:         "https://www.googleapis.com/storage/v1/modules/foomodule.zip",
			expectedSourceURL: "https://www.googleapis.com/storage/v1/modules/foomodule.zip",
		},
		{
			sourceURL:         "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip",
			expectedSourceURL: "gcs::https://www.googleapis.com/storage/v1/modules/foomodule.zip",
		},
		{
			sourceURL:         "git::https://name@dev.azure.com/name/project-name/_git/repo-name",
			expectedSourceURL: "git::https://name@dev.azure.com/name/project-name/_git/repo-name",
		},
		{
			sourceURL:         "https://repository.rnd.net/artifactory/generic-production-iac/tf-auto-azr-iam.2.6.0.zip",
			expectedSourceURL: "https://repository.rnd.net/artifactory/generic-production-iac/tf-auto-azr-iam.2.6.0.zip",
		},
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

// TestEncodeSourceVersionTracksOnlyCopiedFiles pins the local-source version
// hash to the files a copy would deliver: changes to files the copy skips
// (hidden files, exclude_from_copy matches) must not flip the version, while
// changes to copied files (including include_in_copy resurrections) must.
func TestEncodeSourceVersionTracksOnlyCopiedFiles(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-24 * time.Hour)

	touch := func(name string) func(t *testing.T, sourceDir string) {
		return func(t *testing.T, sourceDir string) {
			t.Helper()

			now := time.Now()
			require.NoError(t, os.Chtimes(filepath.Join(sourceDir, name), now, now))
		}
	}

	create := func(name string) func(t *testing.T, sourceDir string) {
		return func(t *testing.T, sourceDir string) {
			t.Helper()

			path := filepath.Join(sourceDir, name)
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
			require.NoError(t, os.WriteFile(path, []byte(name), 0644))
		}
	}

	testCases := []struct {
		mutate   func(t *testing.T, sourceDir string)
		name     string
		copyOpts []util.CopyOption
		wantSame bool
	}{
		{
			name:     "hidden file creation ignored",
			mutate:   create(".this_file_does_not_matter"),
			wantSame: true,
		},
		{
			name:     "hidden file touch ignored",
			mutate:   touch(".hidden.txt"),
			wantSame: true,
		},
		{
			name:     "file in hidden dir ignored",
			mutate:   create(".cache/entry.txt"),
			wantSame: true,
		},
		{
			name:     "tracked file touch detected",
			mutate:   touch("main.tf"),
			wantSame: false,
		},
		{
			name:     "tracked file creation detected",
			mutate:   create("outputs.tf"),
			wantSame: false,
		},
		{
			name:     "exclude_from_copy touch ignored",
			copyOpts: []util.CopyOption{util.WithExcludeFromCopy("ignored.txt")},
			mutate:   touch("ignored.txt"),
			wantSame: true,
		},
		{
			name:     "include_in_copy hidden file touch detected",
			copyOpts: []util.CopyOption{util.WithIncludeInCopy(".hidden.txt")},
			mutate:   touch(".hidden.txt"),
			wantSame: false,
		},
	}

	for _, fastCopy := range []bool{false, true} {
		for _, tc := range testCases {
			name := tc.name
			if fastCopy {
				name += " with fast-copy"
			}

			t.Run(name, func(t *testing.T) {
				t.Parallel()

				l := logger.CreateLogger()
				sourceDir := t.TempDir()

				for _, name := range []string{"main.tf", "ignored.txt", ".hidden.txt"} {
					path := filepath.Join(sourceDir, name)
					require.NoError(t, os.WriteFile(path, []byte(name), 0644))
					require.NoError(t, os.Chtimes(path, past, past))
				}

				copyOpts := tc.copyOpts
				if fastCopy {
					copyOpts = slices.Concat(copyOpts, []util.CopyOption{util.WithFastCopy()})
				}

				src, err := tf.NewSource(l, sourceDir, t.TempDir(), sourceDir, false)
				require.NoError(t, err)

				before, err := src.EncodeSourceVersion(l, copyOpts...)
				require.NoError(t, err)

				tc.mutate(t, sourceDir)

				after, err := src.EncodeSourceVersion(l, copyOpts...)
				require.NoError(t, err)

				if tc.wantSame {
					assert.Equal(t, before, after)
					return
				}

				assert.NotEqual(t, before, after)
			})
		}
	}
}

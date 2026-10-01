package services_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/tf/getproviders"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestRemoveStaleSymlink(t *testing.T) {
	t.Parallel()

	const path = "/cache/registry.terraform.io/hashicorp/aws/5.31.0/linux_amd64"

	testCases := []struct {
		setup     func(t *testing.T, fsys vfs.FS)
		assertErr func(t *testing.T, err error)
		assertFS  func(t *testing.T, fsys vfs.FS)
		name      string
	}{
		{
			name:  "no entry returns nil",
			setup: func(t *testing.T, fsys vfs.FS) { t.Helper() },
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				_, err := vfs.Lstat(fsys, path)
				assert.ErrorIs(t, err, os.ErrNotExist, "expected NotExist, got %v", err)
			},
		},
		{
			name: "dangling symlink is removed",
			setup: func(t *testing.T, fsys vfs.FS) {
				t.Helper()
				require.NoError(t, vfs.Symlink(fsys, "/missing/target", path))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()
				require.NoError(t, err)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				_, err := vfs.Lstat(fsys, path)
				assert.ErrorIs(
					t,
					err,
					os.ErrNotExist,
					"expected NotExist after remove, got %v",
					err,
				)
			},
		},
		{
			name: "regular file returns typed error and is left in place",
			setup: func(t *testing.T, fsys vfs.FS) {
				t.Helper()
				require.NoError(
					t,
					fsys.MkdirAll("/cache/registry.terraform.io/hashicorp/aws/5.31.0", 0o755),
				)
				require.NoError(t, vfs.WriteFile(fsys, path, []byte("user content"), 0o644))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unexpected *services.UnexpectedProviderCachePathError

				require.ErrorAs(t, err, &unexpected)
				assert.Equal(t, path, unexpected.Path)
				assert.Zero(t, unexpected.Mode&os.ModeSymlink)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				exists, err := vfs.FileExists(fsys, path)
				require.NoError(t, err)
				assert.True(t, exists, "regular file must not be deleted")
			},
		},
		{
			name: "regular directory returns typed error and is left in place",
			setup: func(t *testing.T, fsys vfs.FS) {
				t.Helper()
				require.NoError(t, fsys.MkdirAll(path, 0o755))
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unexpected *services.UnexpectedProviderCachePathError

				require.ErrorAs(t, err, &unexpected)
				assert.Equal(t, path, unexpected.Path)
				assert.True(t, unexpected.Mode.IsDir())
				assert.Zero(t, unexpected.Mode&os.ModeSymlink)
			},
			assertFS: func(t *testing.T, fsys vfs.FS) {
				t.Helper()

				exists, err := vfs.FileExists(fsys, path)
				require.NoError(t, err)
				assert.True(t, exists, "directory must not be deleted")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fsys := vfs.NewMemMapFS()
			tc.setup(t, fsys)

			tc.assertErr(t, services.RemoveStaleSymlink(fsys, path))
			tc.assertFS(t, fsys)
		})
	}
}

func TestRemoveStaleSymlinkLstatErrorIsWrapped(t *testing.T) {
	t.Parallel()

	wantInner := errors.New("synthetic lstat failure")
	fsys := &lstatErrorFS{FS: vfs.NewMemMapFS(), err: wantInner}

	err := services.RemoveStaleSymlink(fsys, "/anything")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantInner)
}

type lstatErrorFS struct {
	vfs.FS
	err error
}

func (fsys *lstatErrorFS) LstatIfPossible(string) (os.FileInfo, bool, error) {
	return nil, false, fsys.err
}

// TestProviderServiceInitIsIdempotent covers the server initializing a
// service that a caller may also drive through Run: the directories are
// created once, and both callers see the same outcome.
func TestProviderServiceInitIsIdempotent(t *testing.T) {
	t.Parallel()

	service := services.NewProviderService(
		t.TempDir(), t.TempDir(), nil, logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(),
	)

	require.NoError(t, service.Init())
	require.NoError(t, service.Init())
}

func TestProviderServiceInitWithoutCacheDir(t *testing.T) {
	t.Parallel()

	service := services.NewProviderService(
		"", t.TempDir(), nil, logger.CreateLogger(), venvtest.NewOSWithEmptyEnv(),
	)

	require.ErrorIs(t, service.Init(), services.ErrCacheDirNotSpecified)
	require.ErrorIs(t, service.Run(t.Context()), services.ErrCacheDirNotSpecified)
}

// TestProviderServiceVerifiesRegistryPackages pins which download responses
// the service caches a provider from. A registry's response has to name the
// archive's checksum and the document listing it, and the archive has to match
// that checksum. A mirror's package is cached on its download URL alone.
func TestProviderServiceVerifiesRegistryPackages(t *testing.T) {
	t.Parallel()

	const (
		releasesURL = "https://releases.example.com"
		filename    = "terraform-provider-example_1.0.0_linux_amd64.zip"
		binaryName  = "terraform-provider-example_v1.0.0"
		archivePath = "/" + filename
		shasumsPath = "/terraform-provider-example_1.0.0_SHA256SUMS"
		requestID   = "request"
	)

	archive := providerArchive(t, binaryName)
	archiveSum := sha256.Sum256(archive)
	shasum := hex.EncodeToString(archiveSum[:])

	otherSum := sha256.Sum256([]byte("a different archive"))
	otherShasum := hex.EncodeToString(otherSum[:])

	missingField := func(field services.ChecksumField) func(t *testing.T, err error) {
		return func(t *testing.T, err error) {
			t.Helper()

			var missing *services.ChecksumFieldMissingError

			require.ErrorAs(t, err, &missing)
			assert.Equal(t, field, missing.Field)
		}
	}

	invalidChecksum := func(t *testing.T, err error) {
		t.Helper()

		var invalid *services.InvalidChecksumError

		require.ErrorAs(t, err, &invalid)
	}

	noError := func(t *testing.T, err error) {
		t.Helper()
		require.NoError(t, err)
	}

	testCases := []struct {
		assertErr  func(t *testing.T, err error)
		name       string
		body       models.ResponseBody
		wantCached bool
	}{
		{
			name: "registry response with a checksum and a checksum document",
			body: models.ResponseBody{
				SHA256Sum:     shasum,
				SHA256SumsURL: releasesURL + shasumsPath,
			},
			assertErr:  noError,
			wantCached: true,
		},
		{
			name: "registry response without shasums_url",
			body: models.ResponseBody{
				SHA256Sum: shasum,
			},
			assertErr: missingField(services.FieldSHA256SumsURL),
		},
		{
			name: "registry response without shasum",
			body: models.ResponseBody{
				SHA256SumsURL: releasesURL + shasumsPath,
			},
			assertErr: missingField(services.FieldSHA256Sum),
		},
		{
			name:      "registry response without any checksum field",
			assertErr: missingField(services.FieldSHA256Sum),
		},
		{
			name: "registry response with signing keys and no shasums_signature_url",
			body: models.ResponseBody{
				SHA256Sum:     shasum,
				SHA256SumsURL: releasesURL + shasumsPath,
				SigningKeys: models.SigningKeyList{
					GPGPublicKeys: []*models.SigningKey{{ASCIIArmor: "signing key"}},
				},
			},
			assertErr: missingField(services.FieldSHA256SumsSignatureURL),
		},
		{
			name: "registry response with a shasum longer than a SHA-256 hash",
			body: models.ResponseBody{
				SHA256Sum:     shasum + "00",
				SHA256SumsURL: releasesURL + shasumsPath,
			},
			assertErr: invalidChecksum,
		},
		{
			name: "registry response with a shasum that is not hex",
			body: models.ResponseBody{
				SHA256Sum:     strings.Repeat("z", len(shasum)),
				SHA256SumsURL: releasesURL + shasumsPath,
			},
			assertErr: invalidChecksum,
		},
		{
			name: "registry response whose shasum is for a different archive",
			body: models.ResponseBody{
				SHA256Sum:     otherShasum,
				SHA256SumsURL: releasesURL + shasumsPath,
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var mismatch *getproviders.ArchiveChecksumMismatchError

				require.ErrorAs(t, err, &mismatch)
			},
		},
		{
			name: "mirror package without checksum fields",
			body: models.ResponseBody{
				Origin: models.OriginMirror,
			},
			assertErr:  noError,
			wantCached: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body := tc.body
			body.Filename = filename
			body.DownloadURL = releasesURL + archivePath

			c := vhttp.NewMemClient(
				func(_ context.Context, req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case archivePath:
						return vhttp.Respond(http.StatusOK, archive, nil), nil
					case shasumsPath:
						return vhttp.Respond(
							http.StatusOK,
							fmt.Appendf(nil, "%s  %s\n", body.SHA256Sum, filename),
							nil,
						), nil
					}

					return vhttp.Respond(http.StatusNotFound, nil, nil), nil
				},
			)

			tempDir := t.TempDir()

			service := services.NewProviderService(
				helpers.TmpDirWOSymlinks(t),
				"",
				nil,
				logger.CreateLogger(),
				venvtest.NewOSWithEmptyEnv().
					WithHTTP(c).
					WithTempDir(func() string { return tempDir }),
			)
			require.NoError(t, service.Init())

			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)

			runErr := make(chan error, 1)

			go func() { runErr <- service.Run(ctx) }()

			cache := service.CacheProvider(ctx, requestID, &models.Provider{
				ResponseBody: &body,
				RegistryName: "registry.example.com",
				Namespace:    "example",
				Name:         "example",
				Version:      "1.0.0",
				OS:           "linux",
				Arch:         "amd64",
			})

			_, err := service.WaitForCacheReady(requestID)
			tc.assertErr(t, err)

			cancel()
			tc.assertErr(t, <-runErr)

			if !tc.wantCached {
				assert.NoDirExists(t, cache.PackageDir())

				return
			}

			assert.FileExists(t, filepath.Join(cache.PackageDir(), binaryName))
		})
	}
}

// providerArchive builds a zip archive holding a single file, standing in for
// a provider release archive.
func providerArchive(t *testing.T, name string) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	w, err := zw.Create(name)
	require.NoError(t, err)

	_, err = w.Write([]byte("provider binary"))
	require.NoError(t, err)
	require.NoError(t, zw.Close())

	return buf.Bytes()
}

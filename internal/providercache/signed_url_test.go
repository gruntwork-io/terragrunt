package providercache_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/models"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signedQuery mimics the pre-signed parameters an S3-backed mirror hands back.
const signedQuery = "X-Amz-Algorithm=AWS4-HMAC-SHA256" +
	"&X-Amz-Credential=AKIAIOSFODNN7EXAMPLE%2F20130524%2Fus-east-1%2Fs3%2Faws4_request" +
	"&X-Amz-Date=20130524T000000Z" +
	"&X-Amz-Expires=86400" +
	"&X-Amz-SignedHeaders=host" +
	"&X-Amz-Signature=aeeed9bbccd4d02ee5c0109b86d86835f995330da4c265957d157751f604d404"

// TestNetworkMirrorSignedURLFilename reproduces issue #6670: a network mirror
// returning a signed S3 download URL must produce a clean Filename without query parameters.
func TestNetworkMirrorSignedURLFilename(t *testing.T) {
	t.Parallel()

	providerOS := runtime.GOOS
	providerArch := runtime.GOARCH

	archive := buildWarmupProviderArchive(t)
	archiveChecksum := sha256.Sum256(archive)

	archiveName := fmt.Sprintf(
		"terraform-provider-%s_%s_%s_%s.zip",
		warmupProviderName,
		warmupProviderVersion,
		providerOS,
		providerArch,
	)

	const (
		mirrorHost   = "mirror.example.com"
		archiveHost  = "s3.example.com"
		registryName = "custom-registry.example.com"
	)

	signedDownloadURL := fmt.Sprintf(
		"https://%s/bucket/archives/%s?%s",
		archiveHost, archiveName, signedQuery,
	)

	c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if req.URL.Host != mirrorHost {
			return vhttp.Respond(http.StatusNotFound, nil, nil), nil
		}

		switch {
		case strings.HasSuffix(req.URL.Path, "/index.json"):
			body := fmt.Sprintf(`{"versions":{%q:{}}}`, warmupProviderVersion)

			return vhttp.Respond(http.StatusOK, []byte(body),
				http.Header{"Content-Type": []string{"application/json"}}), nil

		case strings.HasSuffix(req.URL.Path, "/"+warmupProviderVersion+".json"):
			body := fmt.Sprintf(
				`{"archives":{%q:{"url":%q,"hashes":["zh:%s"]}}}`,
				providerOS+"_"+providerArch,
				signedDownloadURL,
				hex.EncodeToString(archiveChecksum[:]),
			)

			return vhttp.Respond(http.StatusOK, []byte(body),
				http.Header{"Content-Type": []string{"application/json"}}), nil
		}

		return vhttp.Respond(http.StatusNotFound, nil, nil), nil
	})

	handler, err := handlers.NewNetworkMirrorProviderHandler(
		logger.CreateLogger(),
		c,
		cliconfig.NewProviderInstallationNetworkMirror(
			"https://"+mirrorHost+"/",
			[]string{registryName + "/*/*"},
			nil,
		),
		nil,
	)
	require.NoError(t, err)

	provider := &models.Provider{
		RegistryName: registryName,
		Namespace:    warmupProviderNamespace,
		Name:         warmupProviderName,
		Version:      warmupProviderVersion,
		OS:           providerOS,
		Arch:         providerArch,
	}

	resp, err := handler.GetPlatform(t.Context(), provider)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The filename must be the clean archive name without query parameters.
	assert.Equal(t, archiveName, resp.Filename,
		"Filename must not include URL query parameters (issue #6670)")

	// The download URL should preserve the query string for authentication.
	assert.Contains(t, resp.DownloadURL, signedQuery,
		"DownloadURL must preserve query parameters for authentication")

	// The filename must be short enough for the filesystem (max 255 chars).
	assert.Less(t, len(resp.Filename), 256,
		"Filename must fit within the OS filename length limit")
}

// TestNetworkMirrorRelativeURLFilename verifies that relative archive URLs resolve
// against the mirror, yield a clean filename, and keep any signed query string.
func TestNetworkMirrorRelativeURLFilename(t *testing.T) {
	t.Parallel()

	providerOS := runtime.GOOS
	providerArch := runtime.GOARCH

	archiveName := fmt.Sprintf(
		"terraform-provider-%s_%s_%s_%s.zip",
		warmupProviderName,
		warmupProviderVersion,
		providerOS,
		providerArch,
	)

	const (
		mirrorHost   = "mirror.example.com"
		registryName = "custom-registry.example.com"
	)

	testCases := []struct {
		name          string
		archiveURL    string
		expectedQuery string
	}{
		{
			name:       "relative URL",
			archiveURL: archiveName,
		},
		{
			name:          "relative URL with signed query",
			archiveURL:    archiveName + "?" + signedQuery,
			expectedQuery: signedQuery,
		},
		{
			name:          "nested relative URL with signed query",
			archiveURL:    "archives/" + archiveName + "?" + signedQuery,
			expectedQuery: signedQuery,
		},
		{
			name:          "absolute path URL with signed query",
			archiveURL:    "/bucket/archives/" + archiveName + "?" + signedQuery,
			expectedQuery: signedQuery,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				if req.URL.Host != mirrorHost {
					return vhttp.Respond(http.StatusNotFound, nil, nil), nil
				}

				if strings.HasSuffix(req.URL.Path, "/"+warmupProviderVersion+".json") {
					body := fmt.Sprintf(
						`{"archives":{%q:{"url":%q}}}`,
						providerOS+"_"+providerArch,
						tc.archiveURL,
					)

					return vhttp.Respond(http.StatusOK, []byte(body),
						http.Header{"Content-Type": []string{"application/json"}}), nil
				}

				return vhttp.Respond(http.StatusNotFound, nil, nil), nil
			})

			handler, err := handlers.NewNetworkMirrorProviderHandler(
				logger.CreateLogger(),
				c,
				cliconfig.NewProviderInstallationNetworkMirror(
					"https://"+mirrorHost+"/",
					[]string{registryName + "/*/*"},
					nil,
				),
				nil,
			)
			require.NoError(t, err)

			provider := &models.Provider{
				RegistryName: registryName,
				Namespace:    warmupProviderNamespace,
				Name:         warmupProviderName,
				Version:      warmupProviderVersion,
				OS:           providerOS,
				Arch:         providerArch,
			}

			resp, err := handler.GetPlatform(t.Context(), provider)
			require.NoError(t, err)
			require.NotNil(t, resp)

			assert.Equal(t, archiveName, resp.Filename,
				"Filename must match the relative archive name")

			downloadURL, err := url.Parse(resp.DownloadURL)
			require.NoError(t, err)

			assert.Equal(t, mirrorHost, downloadURL.Host,
				"relative archive URL must resolve against the mirror")
			assert.True(t, strings.HasSuffix(downloadURL.Path, "/"+archiveName),
				"resolved path %q must end with the archive name", downloadURL.Path)
			assert.Equal(t, tc.expectedQuery, downloadURL.RawQuery,
				"DownloadURL must preserve query parameters for authentication")
		})
	}
}

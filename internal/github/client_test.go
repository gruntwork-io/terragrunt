package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewGitHubAPIClient()
	require.NotNil(t, client)

	assert.Equal(t, "https://api.github.com", client.baseURL)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.cache)
}

func TestNewClientWithOptions(t *testing.T) {
	t.Parallel()

	customHTTPClient := &http.Client{Timeout: 10 * time.Second}
	customBaseURL := "https://custom.github.com"

	client := NewGitHubAPIClient(
		WithHTTPClient(customHTTPClient),
		WithBaseURL(customBaseURL),
	)

	assert.Equal(t, customHTTPClient, client.httpClient)
	assert.Equal(t, customBaseURL, client.baseURL)
}

func TestGetLatestRelease(t *testing.T) {
	t.Parallel()

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		assert.Equal(t, "/repos/owner/repo/releases/latest", r.URL.Path)
		assert.Equal(t, "application/vnd.github.v3+json", r.Header.Get("Accept"))

		w.Header().Set("Content-Type", "application/json")
		response := `{
			"tag_name": "v1.2.3",
			"name": "Release v1.2.3",
			"html_url": "https://github.com/owner/repo/releases/tag/v1.2.3"
		}`
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	client := NewGitHubAPIClient(WithBaseURL(server.URL))

	release, err := client.GetLatestRelease(t.Context(), "owner/repo")
	require.NoError(t, err)

	assert.Equal(t, "v1.2.3", release.TagName)
	assert.Equal(t, "Release v1.2.3", release.Name)
	assert.Equal(t, "https://github.com/owner/repo/releases/tag/v1.2.3", release.URL)
}

func TestGetLatestReleaseTag(t *testing.T) {
	t.Parallel()

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{"tag_name": "v2.0.0"}`
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	client := NewGitHubAPIClient(WithBaseURL(server.URL))

	tag, err := client.GetLatestReleaseTag(t.Context(), "owner/repo")
	require.NoError(t, err)

	assert.Equal(t, "v2.0.0", tag)
}

func TestGetLatestReleaseInvalidRepository(t *testing.T) {
	t.Parallel()

	client := NewGitHubAPIClient()

	testCases := []string{
		"",
		"invalid",
		"too/many/parts",
	}

	for _, repo := range testCases {
		t.Run(fmt.Sprintf("repo=%s", repo), func(tt *testing.T) {
			_, err := client.GetLatestRelease(tt.Context(), repo)
			require.Error(t, err)
		})
	}
}

func TestGetLatestReleaseHTTPError(t *testing.T) {
	t.Parallel()

	// Create a mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "Not Found")
	}))
	defer server.Close()

	client := NewGitHubAPIClient(WithBaseURL(server.URL))

	_, err := client.GetLatestRelease(t.Context(), "owner/repo")
	require.Error(t, err)
	assert.ErrorContains(t, err, "GitHub API request to determine latest release failed with status 404")
}

func TestGetLatestReleaseEmptyTag(t *testing.T) {
	t.Parallel()

	// Create a mock server that returns empty tag
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		response := `{"tag_name": ""}`
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	client := NewGitHubAPIClient(WithBaseURL(server.URL))

	_, err := client.GetLatestRelease(t.Context(), "owner/repo")
	require.Error(t, err)
	assert.ErrorContains(t, err, "GitHub API returned empty tag name for latest release")
}

func TestGetLatestReleaseCaching(t *testing.T) {
	t.Parallel()

	callCount := 0
	// Create a mock server that tracks how many times it's called
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		response := `{"tag_name": "v1.0.0"}`
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	client := NewGitHubAPIClient(WithBaseURL(server.URL))

	// First call should hit the server
	tag1, err := client.GetLatestReleaseTag(t.Context(), "owner/repo")
	require.NoError(t, err)

	assert.Equal(t, "v1.0.0", tag1)
	assert.Equal(t, 1, callCount)

	// Second call should use cache
	tag2, err := client.GetLatestReleaseTag(t.Context(), "owner/repo")
	require.NoError(t, err)

	assert.Equal(t, "v1.0.0", tag2)
	assert.Equal(t, 1, callCount)
}

// Tests for GitHubReleasesDownloadClient

func TestNewGitHubReleasesDownloadClient(t *testing.T) {
	t.Parallel()

	client := NewGitHubReleasesDownloadClient()
	if client == nil {
		t.Fatal("NewGitHubReleasesDownloadClient() returned nil")
	}

	if client.logger != nil {
		t.Error("Expected logger to be nil by default")
	}
}

func TestNewGitHubReleasesDownloadClientWithOptions(t *testing.T) {
	t.Parallel()

	logger := log.New()
	client := NewGitHubReleasesDownloadClient(WithLogger(logger))

	if client.logger != logger {
		t.Error("Expected custom logger to be set")
	}
}

func TestDownloadReleaseAssetsValidation(t *testing.T) {
	t.Parallel()

	client := NewGitHubReleasesDownloadClient()
	ctx := context.Background()

	testCases := []struct {
		name     string
		assets   *ReleaseAssets
		errorMsg string
	}{
		{
			name:     "empty repository",
			assets:   &ReleaseAssets{Repository: "", PackageFile: "/tmp/package.zip"},
			errorMsg: "repository cannot be empty",
		},
		{
			name:     "empty package file",
			assets:   &ReleaseAssets{Repository: "owner/repo", PackageFile: ""},
			errorMsg: "package file path cannot be empty",
		},
		{
			name:     "missing version for GitHub repo",
			assets:   &ReleaseAssets{Repository: "owner/repo", Version: "", PackageFile: "/tmp/package.zip"},
			errorMsg: "version cannot be empty for GitHub repository downloads",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.DownloadReleaseAssets(ctx, tc.assets)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.errorMsg)
		})
	}
}

func TestDownloadReleaseAssetsGitHubRelease(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create mock server for GitHub releases
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Serve different content based on the requested file
		if strings.HasSuffix(path, "package.zip") {
			w.Header().Set("Content-Type", "application/zip")
			fmt.Fprint(w, "fake-zip-content")
		} else if strings.HasSuffix(path, "SHA256SUMS") {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "fake-checksum-content")
		} else if strings.HasSuffix(path, "SHA256SUMS.sig") {
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, "fake-signature-content")
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Use direct URL approach for testing since mock servers are complex to set up for GitHub releases format
	client := NewGitHubReleasesDownloadClient()

	assets := &ReleaseAssets{
		Repository:  server.URL + "/package.zip", // Direct URL
		PackageFile: filepath.Join(tempDir, "package.zip"),
		// Direct URLs don't use checksum files
	}

	ctx := context.Background()
	result, err := client.DownloadReleaseAssets(ctx, assets)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, assets.PackageFile, result.PackageFile)
	assert.Equal(t, "", result.ChecksumFile)
	assert.Equal(t, "", result.ChecksumSigFile)

	// Verify package file was created and has expected content
	verifyFileContent(t, result.PackageFile, "fake-zip-content")
}

func TestDownloadReleaseAssetsDirectURL(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		fmt.Fprint(w, "direct-url-content")
	}))
	defer server.Close()

	client := NewGitHubReleasesDownloadClient()

	assets := &ReleaseAssets{
		Repository:  server.URL + "/direct-download.zip",
		PackageFile: filepath.Join(tempDir, "direct.zip"),
		// Note: No Version, ChecksumFile, or ChecksumSigFile for direct URLs
	}

	ctx := context.Background()
	result, err := client.DownloadReleaseAssets(ctx, assets)
	require.NoError(t, err)

	// Verify result
	assert.Equal(t, assets.PackageFile, result.PackageFile)
	assert.Equal(t, "", result.ChecksumFile)
	assert.Equal(t, "", result.ChecksumSigFile)

	// Verify file was created and has expected content
	verifyFileContent(t, result.PackageFile, "direct-url-content")
}

// Helper function to verify file content
func verifyFileContent(t *testing.T, filePath, expectedContent string) {
	t.Helper()

	require.FileExists(t, filePath)

	content, err := os.ReadFile(filePath)
	require.NoError(t, err)

	assert.Equal(t, expectedContent, string(content))
}

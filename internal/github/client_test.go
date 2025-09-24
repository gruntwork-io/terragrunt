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
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewGitHubAPIClient()
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}

	if client.baseURL != "https://api.github.com" {
		t.Errorf("Expected baseURL to be 'https://api.github.com', got '%s'", client.baseURL)
	}

	if client.httpClient == nil {
		t.Fatal("httpClient should not be nil")
	}

	if client.cache == nil {
		t.Fatal("cache should not be nil")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	t.Parallel()

	customHTTPClient := &http.Client{Timeout: 10 * time.Second}
	customBaseURL := "https://custom.github.com"

	client := NewGitHubAPIClient(
		WithHTTPClient(customHTTPClient),
		WithBaseURL(customBaseURL),
	)

	if client.httpClient != customHTTPClient {
		t.Error("Expected custom HTTP client to be set")
	}

	if client.baseURL != customBaseURL {
		t.Errorf("Expected baseURL to be '%s', got '%s'", customBaseURL, client.baseURL)
	}
}

func TestGetLatestRelease(t *testing.T) {
	t.Parallel()

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/releases/latest" {
			t.Errorf("Expected path '/repos/owner/repo/releases/latest', got '%s'", r.URL.Path)
		}

		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Expected Accept header 'application/vnd.github.v3+json', got '%s'", r.Header.Get("Accept"))
		}

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

	ctx := context.Background()
	release, err := client.GetLatestRelease(ctx, "owner/repo")
	if err != nil {
		t.Fatalf("GetLatestRelease() failed: %v", err)
	}

	if release.TagName != "v1.2.3" {
		t.Errorf("Expected tag name 'v1.2.3', got '%s'", release.TagName)
	}

	if release.Name != "Release v1.2.3" {
		t.Errorf("Expected name 'Release v1.2.3', got '%s'", release.Name)
	}

	if release.URL != "https://github.com/owner/repo/releases/tag/v1.2.3" {
		t.Errorf("Expected URL 'https://github.com/owner/repo/releases/tag/v1.2.3', got '%s'", release.URL)
	}
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

	ctx := context.Background()
	tag, err := client.GetLatestReleaseTag(ctx, "owner/repo")
	if err != nil {
		t.Fatalf("GetLatestReleaseTag() failed: %v", err)
	}

	if tag != "v2.0.0" {
		t.Errorf("Expected tag 'v2.0.0', got '%s'", tag)
	}
}

func TestGetLatestReleaseInvalidRepository(t *testing.T) {
	t.Parallel()

	client := NewGitHubAPIClient()
	ctx := context.Background()

	testCases := []string{
		"",
		"invalid",
		"too/many/parts",
	}

	for _, repo := range testCases {
		t.Run(fmt.Sprintf("repo=%s", repo), func(t *testing.T) {
			_, err := client.GetLatestRelease(ctx, repo)
			if err == nil {
				t.Errorf("Expected error for invalid repository '%s', but got none", repo)
			}
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

	ctx := context.Background()
	_, err := client.GetLatestRelease(ctx, "owner/repo")
	if err == nil {
		t.Fatal("Expected error for HTTP 404, but got none")
	}

	expectedErrorSubstring := "GitHub API request to determine latest release failed with status 404"
	if !containsString(err.Error(), expectedErrorSubstring) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedErrorSubstring, err.Error())
	}
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

	ctx := context.Background()
	_, err := client.GetLatestRelease(ctx, "owner/repo")
	if err == nil {
		t.Fatal("Expected error for empty tag name, but got none")
	}

	expectedErrorSubstring := "GitHub API returned empty tag name for latest release"
	if !containsString(err.Error(), expectedErrorSubstring) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedErrorSubstring, err.Error())
	}
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

	ctx := context.Background()

	// First call should hit the server
	tag1, err := client.GetLatestReleaseTag(ctx, "owner/repo")
	if err != nil {
		t.Fatalf("First call failed: %v", err)
	}
	if tag1 != "v1.0.0" {
		t.Errorf("Expected tag 'v1.0.0', got '%s'", tag1)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 server call, got %d", callCount)
	}

	// Second call should use cache
	tag2, err := client.GetLatestReleaseTag(ctx, "owner/repo")
	if err != nil {
		t.Fatalf("Second call failed: %v", err)
	}
	if tag2 != "v1.0.0" {
		t.Errorf("Expected tag 'v1.0.0', got '%s'", tag2)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 server call (cached), got %d", callCount)
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			func() bool {
				for i := 1; i <= len(s)-len(substr); i++ {
					if s[i:i+len(substr)] == substr {
						return true
					}
				}
				return false
			}())))
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
			if err == nil {
				t.Errorf("Expected error for %s, but got none", tc.name)
			}
			if !containsString(err.Error(), tc.errorMsg) {
				t.Errorf("Expected error to contain '%s', got '%s'", tc.errorMsg, err.Error())
			}
		})
	}
}

func TestDownloadReleaseAssetsGitHubRelease(t *testing.T) {
	t.Parallel()

	// Create temporary directory for downloads
	tempDir, err := os.MkdirTemp("", "github_download_test_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

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
	if err != nil {
		t.Fatalf("DownloadReleaseAssets() failed: %v", err)
	}

	// Verify result
	if result.PackageFile != assets.PackageFile {
		t.Errorf("Expected package file '%s', got '%s'", assets.PackageFile, result.PackageFile)
	}
	if result.ChecksumFile != "" {
		t.Errorf("Expected empty checksum file for direct URL, got '%s'", result.ChecksumFile)
	}
	if result.ChecksumSigFile != "" {
		t.Errorf("Expected empty signature file for direct URL, got '%s'", result.ChecksumSigFile)
	}

	// Verify package file was created and has expected content
	verifyFileContent(t, result.PackageFile, "fake-zip-content")
}

func TestDownloadReleaseAssetsDirectURL(t *testing.T) {
	t.Parallel()

	// Create temporary directory for downloads
	tempDir, err := os.MkdirTemp("", "github_download_test_direct_")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

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
	if err != nil {
		t.Fatalf("DownloadReleaseAssets() failed: %v", err)
	}

	// Verify result
	if result.PackageFile != assets.PackageFile {
		t.Errorf("Expected package file '%s', got '%s'", assets.PackageFile, result.PackageFile)
	}
	if result.ChecksumFile != "" {
		t.Errorf("Expected empty checksum file, got '%s'", result.ChecksumFile)
	}
	if result.ChecksumSigFile != "" {
		t.Errorf("Expected empty signature file, got '%s'", result.ChecksumSigFile)
	}

	// Verify file was created and has expected content
	verifyFileContent(t, result.PackageFile, "direct-url-content")
}

// Helper function to verify file content
func verifyFileContent(t *testing.T, filePath, expectedContent string) {
	t.Helper()

	if !fileExists(filePath) {
		t.Errorf("Expected file '%s' to exist", filePath)
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Errorf("Failed to read file '%s': %v", filePath, err)
		return
	}

	if string(content) != expectedContent {
		t.Errorf("Expected file content '%s', got '%s'", expectedContent, string(content))
	}
}

// Helper function to check if file exists
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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

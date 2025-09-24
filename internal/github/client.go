// Package github provides a client for interacting with the GitHub API.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// GitHubAPIClient represents a GitHub API client.
type GitHubAPIClient struct {
	baseURL    string
	httpClient *http.Client
	cache      *cache.ExpiringCache[string]
}

// Release represents a GitHub repository release.
type Release struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	URL     string `json:"html_url"`
}

// GitHubAPIClientOption is a function that configures a GitHubAPIClient.
type GitHubAPIClientOption func(*GitHubAPIClient)

// WithHTTPClient sets the HTTP client for the GitHub client.
func WithHTTPClient(httpClient *http.Client) GitHubAPIClientOption {
	return func(c *GitHubAPIClient) {
		c.httpClient = httpClient
	}
}

// WithBaseURL sets the base URL for the GitHub API.
func WithBaseURL(baseURL string) GitHubAPIClientOption {
	return func(c *GitHubAPIClient) {
		c.baseURL = baseURL
	}
}

// NewGitHubAPIClient creates a new GitHub API client with optional configuration.
func NewGitHubAPIClient(opts ...GitHubAPIClientOption) *GitHubAPIClient {
	client := &GitHubAPIClient{
		baseURL:    "https://api.github.com",
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cache:      cache.NewExpiringCache[string]("github_api"),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// GetLatestRelease fetches the latest release for a given repository.
// The repository should be in the format "owner/repo".
func (c *GitHubAPIClient) GetLatestRelease(ctx context.Context, repository string) (*Release, error) {
	if repository == "" {
		return nil, errors.Errorf("repository cannot be empty")
	}

	parts := strings.Split(repository, "/")
	if len(parts) != 2 {
		return nil, errors.Errorf("repository must be in format 'owner/repo', got: %s", repository)
	}

	url := fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, repository)

	if cachedTag, found := c.cache.Get(ctx, url); found {
		return &Release{TagName: cachedTag}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, errors.Errorf(
			"GitHub API request to determine latest release failed with status %d: %s",
			resp.StatusCode,
			resp.Status,
		)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Errorf("failed to read response body: %w", err)
	}

	var release Release
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, errors.Errorf("failed to parse GitHub API response: %w", err)
	}

	if release.TagName == "" {
		return nil, errors.Errorf("GitHub API returned empty tag name for latest release")
	}

	c.cache.Put(ctx, url, release.TagName, time.Now().Add(5*time.Minute))

	return &release, nil
}

// GetLatestReleaseTag is a convenience method that returns just the tag name
// of the latest release for a repository.
func (c *GitHubAPIClient) GetLatestReleaseTag(ctx context.Context, repository string) (string, error) {
	release, err := c.GetLatestRelease(ctx, repository)
	if err != nil {
		return "", err
	}

	return release.TagName, nil
}

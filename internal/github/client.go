// Package github provides clients for interacting with the GitHub API and downloading GitHub releases.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/go-getter"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
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

// GitHubReleasesDownloadClient represents a client for downloading GitHub release assets.
type GitHubReleasesDownloadClient struct {
	logger log.Logger
}

// ReleaseAssets represents the assets to download for a GitHub release.
type ReleaseAssets struct {
	Repository      string
	Version         string
	PackageFile     string
	ChecksumFile    string
	ChecksumSigFile string
}

// DownloadResult represents the result of downloading release assets.
type DownloadResult struct {
	PackageFile     string
	ChecksumFile    string
	ChecksumSigFile string
}

// GitHubReleasesDownloadClientOption is a function that configures a GitHubReleasesDownloadClient.
type GitHubReleasesDownloadClientOption func(*GitHubReleasesDownloadClient)

// WithLogger sets the logger for the download client.
func WithLogger(logger log.Logger) GitHubReleasesDownloadClientOption {
	return func(c *GitHubReleasesDownloadClient) {
		c.logger = logger
	}
}

// NewGitHubReleasesDownloadClient creates a new GitHub releases download client.
func NewGitHubReleasesDownloadClient(opts ...GitHubReleasesDownloadClientOption) *GitHubReleasesDownloadClient {
	client := &GitHubReleasesDownloadClient{}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// DownloadReleaseAssets downloads the specified release assets from a GitHub repository.
// It supports downloading from either full URLs (when repository contains "://") or
// from GitHub releases using the standard GitHub releases URL format.
func (c *GitHubReleasesDownloadClient) DownloadReleaseAssets(
	ctx context.Context,
	assets *ReleaseAssets,
) (*DownloadResult, error) {
	if assets.Repository == "" {
		return nil, errors.Errorf("repository cannot be empty")
	}

	if assets.PackageFile == "" {
		return nil, errors.Errorf("package file path cannot be empty")
	}

	result := &DownloadResult{
		PackageFile: assets.PackageFile,
	}

	expectedLen := 1

	if assets.ChecksumFile != "" {
		expectedLen++
	}

	if assets.ChecksumSigFile != "" {
		expectedLen++
	}

	downloads := make(map[string]string, expectedLen)

	if strings.Contains(assets.Repository, "://") {
		// If repository contains "://", treat it as a direct URL
		downloads[assets.Repository] = assets.PackageFile
	} else {
		if assets.Version == "" {
			return nil, errors.Errorf("version cannot be empty for GitHub repository downloads")
		}

		baseURL := fmt.Sprintf("https://%s/releases/download/%s", assets.Repository, assets.Version)
		packageFileName := filepath.Base(assets.PackageFile)

		downloads[fmt.Sprintf("%s/%s", baseURL, packageFileName)] = assets.PackageFile

		if assets.ChecksumFile != "" {
			checksumFileName := filepath.Base(assets.ChecksumFile)
			downloads[fmt.Sprintf("%s/%s", baseURL, checksumFileName)] = assets.ChecksumFile
			result.ChecksumFile = assets.ChecksumFile
		}

		if assets.ChecksumSigFile != "" {
			checksumSigFileName := filepath.Base(assets.ChecksumSigFile)
			downloads[fmt.Sprintf("%s/%s", baseURL, checksumSigFileName)] = assets.ChecksumSigFile
			result.ChecksumSigFile = assets.ChecksumSigFile
		}
	}

	g, downloadCtx := errgroup.WithContext(ctx)

	for url, localPath := range downloads {
		g.Go(func() error {
			if c.logger != nil {
				c.logger.Infof("Downloading %s to %s", url, localPath)
			}

			client := &getter.Client{
				Ctx:           downloadCtx,
				Src:           url,
				Dst:           localPath,
				Mode:          getter.ClientModeFile,
				Decompressors: map[string]getter.Decompressor{},
			}

			if err := client.Get(); err != nil {
				return errors.Errorf("failed to download %s: %w", url, err)
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

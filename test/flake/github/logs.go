package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// DownloadJobLogs downloads logs for a specific job to the given output path.
func (c *Client) DownloadJobLogs(ctx context.Context, jobID int64, outputPath string) error {
	url, _, err := c.client.Actions.GetWorkflowJobLogs(ctx, c.owner, c.repo, jobID, true)
	if err != nil {
		return fmt.Errorf("failed to get job logs URL: %w", err)
	}

	return downloadFile(ctx, url.String(), outputPath)
}

// DownloadRunLogs downloads all logs for a workflow run to the given directory.
func (c *Client) DownloadRunLogs(ctx context.Context, runID int64, outputDir string) (string, error) {
	url, _, err := c.client.Actions.GetWorkflowRunLogs(ctx, c.owner, c.repo, runID, true)
	if err != nil {
		return "", fmt.Errorf("failed to get run logs URL: %w", err)
	}

	outputPath := filepath.Join(outputDir, fmt.Sprintf("%d_logs.zip", runID))
	if err := downloadFile(ctx, url.String(), outputPath); err != nil {
		return "", err
	}

	return outputPath, nil
}

// downloadFile downloads a file from a URL to the specified path.
func downloadFile(ctx context.Context, url, outputPath string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

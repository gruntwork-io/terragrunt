package github

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v53/github"
)

// Artifact represents a workflow artifact.
type Artifact struct {
	ID   int64
	Name string
}

// ListArtifacts lists artifacts for a workflow run.
func (c *Client) ListArtifacts(ctx context.Context, runID int64) ([]Artifact, error) {
	artifacts, _, err := c.client.Actions.ListWorkflowRunArtifacts(ctx, c.owner, c.repo, runID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list artifacts: %w", err)
	}

	var result []Artifact
	for _, a := range artifacts.Artifacts {
		result = append(result, Artifact{
			ID:   a.GetID(),
			Name: a.GetName(),
		})
	}

	return result, nil
}

// DownloadArtifact downloads and extracts an artifact to the specified directory.
func (c *Client) DownloadArtifact(ctx context.Context, artifactID int64, outputDir string) error {
	url, _, err := c.client.Actions.DownloadArtifact(ctx, c.owner, c.repo, artifactID, true)
	if err != nil {
		return fmt.Errorf("failed to get artifact URL: %w", err)
	}

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "artifact-*.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if err := downloadFile(ctx, url.String(), tmpFile.Name()); err != nil {
		return err
	}

	// Extract zip
	return extractZip(tmpFile.Name(), outputDir)
}

// DownloadTestReportArtifacts downloads all test report artifacts for a run.
func (c *Client) DownloadTestReportArtifacts(ctx context.Context, runID int64, outputDir string) error {
	artifacts, err := c.ListArtifacts(ctx, runID)
	if err != nil {
		return err
	}

	for _, a := range artifacts {
		// Look for test report artifacts
		if strings.Contains(strings.ToLower(a.Name), "test") ||
			strings.Contains(strings.ToLower(a.Name), "report") ||
			strings.Contains(strings.ToLower(a.Name), "result") {
			artifactDir := filepath.Join(outputDir, fmt.Sprintf("%d_%s", runID, sanitizeFilename(a.Name)))
			if err := c.DownloadArtifact(ctx, a.ID, artifactDir); err != nil {
				// Log but continue on artifact download failure
				fmt.Printf("Warning: failed to download artifact %s: %v\n", a.Name, err)
			}
		}
	}

	return nil
}

// GetWorkflowRunSummary retrieves the summary/check run annotations for a workflow run.
func (c *Client) GetWorkflowRunSummary(ctx context.Context, runID int64) (string, error) {
	// Get check runs for the workflow run's commit
	run, _, err := c.client.Actions.GetWorkflowRunByID(ctx, c.owner, c.repo, runID)
	if err != nil {
		return "", fmt.Errorf("failed to get workflow run: %w", err)
	}

	// List check runs for this commit
	checkRuns, _, err := c.client.Checks.ListCheckRunsForRef(ctx, c.owner, c.repo, run.GetHeadSHA(), &github.ListCheckRunsOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list check runs: %w", err)
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("# Workflow Run %d Summary\n\n", runID))
	summary.WriteString(fmt.Sprintf("**Commit:** %s\n", run.GetHeadSHA()))
	summary.WriteString(fmt.Sprintf("**Branch:** %s\n", run.GetHeadBranch()))
	summary.WriteString(fmt.Sprintf("**URL:** %s\n\n", run.GetHTMLURL()))

	for _, cr := range checkRuns.CheckRuns {
		if cr.GetConclusion() == "failure" {
			summary.WriteString(fmt.Sprintf("## %s (Failed)\n\n", cr.GetName()))
			if cr.Output != nil && cr.Output.Summary != nil {
				summary.WriteString(*cr.Output.Summary)
				summary.WriteString("\n\n")
			}
		}
	}

	return summary.String(), nil
}

// extractZip extracts a zip file to the specified directory.
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(fpath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path: %s", fpath)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}

	return nil
}

// sanitizeFilename removes or replaces characters that are invalid in filenames.
func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	return replacer.Replace(name)
}

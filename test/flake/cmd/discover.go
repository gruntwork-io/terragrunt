// Package cmd provides CLI commands for the flake utility.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/test/flake/github"
	"github.com/gruntwork-io/terragrunt/test/flake/types"
	"github.com/urfave/cli/v2"
)

// DiscoverCommand returns the discover CLI command.
func DiscoverCommand() *cli.Command {
	return &cli.Command{
		Name:  "discover",
		Usage: "Discover failed CI runs and download logs",
		Description: `Fetches failed workflow runs from GitHub Actions and downloads
logs and summaries for analysis.

The command will:
  1. List failed workflow runs on the specified branch
  2. For each failed run, identify failed jobs
  3. Download job logs to the logs/ directory
  4. Download workflow summaries to the summaries/ directory
  5. Save a manifest of discovered data

Example:
  flake discover --token $GITHUB_TOKEN --limit 20`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "token",
				Aliases:  []string{"t"},
				Usage:    "GitHub token for API access",
				EnvVars:  []string{"GITHUB_TOKEN"},
				Required: true,
			},
			&cli.StringFlag{
				Name:    "repo",
				Aliases: []string{"r"},
				Usage:   "Repository in owner/repo format",
				Value:   "gruntwork-io/terragrunt",
			},
			&cli.StringFlag{
				Name:    "workflow",
				Aliases: []string{"w"},
				Usage:   "Workflow filename to check",
				Value:   "ci.yml",
			},
			&cli.StringFlag{
				Name:    "branch",
				Aliases: []string{"b"},
				Usage:   "Branch to check for failures",
				Value:   "main",
			},
			&cli.IntFlag{
				Name:    "limit",
				Aliases: []string{"n"},
				Usage:   "Maximum number of failed runs to fetch",
				Value:   20,
			},
			&cli.StringFlag{
				Name:    "since",
				Aliases: []string{"s"},
				Usage:   "Only check runs since this date (YYYY-MM-DD)",
			},
			&cli.StringFlag{
				Name:    "output-dir",
				Aliases: []string{"o"},
				Usage:   "Base output directory",
				Value:   ".",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Enable verbose output",
			},
		},
		Action: runDiscover,
	}
}

func runDiscover(c *cli.Context) error {
	token := c.String("token")
	repoFull := c.String("repo")
	workflow := c.String("workflow")
	branch := c.String("branch")
	limit := c.Int("limit")
	sinceStr := c.String("since")
	outputDir := c.String("output-dir")
	verbose := c.Bool("verbose")

	// Parse repo
	parts := strings.Split(repoFull, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid repo format, expected owner/repo: %s", repoFull)
	}
	owner, repo := parts[0], parts[1]

	// Parse since date
	var since *time.Time
	if sinceStr != "" {
		t, err := time.Parse("2006-01-02", sinceStr)
		if err != nil {
			return fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
		}
		since = &t
	}

	// Create output directories
	logsDir := filepath.Join(outputDir, "logs")
	summariesDir := filepath.Join(outputDir, "summaries")

	for _, dir := range []string{logsDir, summariesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create GitHub client
	client := github.NewClient(token, owner, repo)
	ctx := context.Background()

	// Fetch failed runs
	if verbose {
		fmt.Printf("Fetching failed workflow runs for %s on branch %s...\n", workflow, branch)
	}

	runs, err := client.ListFailedWorkflowRuns(ctx, workflow, branch, limit, since)
	if err != nil {
		return fmt.Errorf("failed to list workflow runs: %w", err)
	}

	fmt.Printf("Found %d failed runs\n", len(runs))

	// Track all jobs
	var allJobs []types.Job

	// Process each run
	for i, run := range runs {
		if verbose {
			fmt.Printf("\n[%d/%d] Processing run #%d (ID: %d)\n", i+1, len(runs), run.RunNumber, run.ID)
		}

		// Get failed jobs
		jobs, err := client.GetFailedJobs(ctx, run.ID)
		if err != nil {
			fmt.Printf("Warning: failed to get jobs for run %d: %v\n", run.ID, err)
			continue
		}

		if verbose {
			fmt.Printf("  Found %d failed jobs\n", len(jobs))
		}

		// Download logs for failed jobs
		for _, job := range jobs {
			logFile := filepath.Join(logsDir, fmt.Sprintf("%d_%s.log", run.ID, sanitizeFilename(job.Name)))

			if verbose {
				fmt.Printf("  Downloading logs for job: %s\n", job.Name)
			}

			if err := client.DownloadJobLogs(ctx, job.ID, logFile); err != nil {
				fmt.Printf("Warning: failed to download logs for job %s: %v\n", job.Name, err)
				continue
			}

			job.LogFile = logFile
			allJobs = append(allJobs, job)
		}

		// Download workflow summary
		summaryFile := filepath.Join(summariesDir, fmt.Sprintf("%d_summary.md", run.ID))
		summary, err := client.GetWorkflowRunSummary(ctx, run.ID)
		if err != nil {
			if verbose {
				fmt.Printf("  Warning: failed to get summary: %v\n", err)
			}
		} else {
			if err := os.WriteFile(summaryFile, []byte(summary), 0644); err != nil {
				fmt.Printf("Warning: failed to write summary: %v\n", err)
			}
		}

		// Rate limit protection
		time.Sleep(100 * time.Millisecond)
	}

	// Save manifest
	manifest := types.DiscoveryManifest{
		LastUpdated: time.Now(),
		Repository:  repoFull,
		Branch:      branch,
		Workflow:    workflow,
		Runs:        runs,
		Jobs:        allJobs,
	}

	manifestPath := filepath.Join(logsDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	fmt.Printf("\nDiscovery complete:\n")
	fmt.Printf("  - Runs processed: %d\n", len(runs))
	fmt.Printf("  - Jobs with logs: %d\n", len(allJobs))
	fmt.Printf("  - Logs directory: %s\n", logsDir)
	fmt.Printf("  - Summaries directory: %s\n", summariesDir)
	fmt.Printf("  - Manifest: %s\n", manifestPath)

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
		"(", "",
		")", "",
	)
	return replacer.Replace(name)
}

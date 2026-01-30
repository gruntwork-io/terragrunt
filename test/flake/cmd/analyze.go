package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/terragrunt/test/flake/analyzer"
	"github.com/gruntwork-io/terragrunt/test/flake/parser"
	"github.com/gruntwork-io/terragrunt/test/flake/types"
	"github.com/urfave/cli/v2"
)

// AnalyzeCommand returns the analyze CLI command.
func AnalyzeCommand() *cli.Command {
	return &cli.Command{
		Name:  "analyze",
		Usage: "Analyze discovered logs and generate reports",
		Description: `Parses downloaded logs to identify test failures and generates
analysis reports in markdown and/or JSON format.

The command will:
  1. Read the discovery manifest from logs/manifest.json
  2. Parse all log files for test failures
  3. Group failures by test name and calculate statistics
  4. Generate reports in the analysis/ directory

Example:
  flake analyze --format both --min-failures 2`,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "input-dir",
				Aliases: []string{"i"},
				Usage:   "Directory containing logs and summaries",
				Value:   ".",
			},
			&cli.StringFlag{
				Name:    "output-dir",
				Aliases: []string{"o"},
				Usage:   "Directory for analysis output",
				Value:   "analysis",
			},
			&cli.IntFlag{
				Name:  "min-failures",
				Usage: "Minimum failures to include in report",
				Value: 1,
			},
			&cli.StringFlag{
				Name:    "format",
				Aliases: []string{"f"},
				Usage:   "Output format: markdown, json, both",
				Value:   "both",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "Enable verbose output",
			},
		},
		Action: runAnalyze,
	}
}

func runAnalyze(c *cli.Context) error {
	inputDir := c.String("input-dir")
	outputDir := c.String("output-dir")
	minFailures := c.Int("min-failures")
	format := c.String("format")
	verbose := c.Bool("verbose")

	// Validate format
	if format != "markdown" && format != "json" && format != "both" {
		return fmt.Errorf("invalid format: %s (must be markdown, json, or both)", format)
	}

	logsDir := filepath.Join(inputDir, "logs")
	manifestPath := filepath.Join(logsDir, "manifest.json")

	// Load manifest
	if verbose {
		fmt.Println("Loading discovery manifest...")
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest (did you run 'flake discover' first?): %w", err)
	}

	var manifest types.DiscoveryManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	if verbose {
		fmt.Printf("Manifest loaded: %d runs, %d jobs\n", len(manifest.Runs), len(manifest.Jobs))
	}

	// Parse all log files
	if verbose {
		fmt.Println("Parsing log files...")
	}

	failures, err := parser.ParseLogsDir(logsDir, &manifest)
	if err != nil {
		return fmt.Errorf("failed to parse logs: %w", err)
	}

	if verbose {
		fmt.Printf("Found %d test failures\n", len(failures))
	}

	// Build report
	report := analyzer.BuildReport(failures, len(manifest.Runs), len(manifest.Runs))
	report.GeneratedAt = time.Now().UTC()

	// Filter by minimum failures
	if minFailures > 1 {
		report.TestStats = analyzer.FilterByMinFailures(report.TestStats, minFailures)
		if verbose {
			fmt.Printf("After filtering (min %d failures): %d unique tests\n", minFailures, len(report.TestStats))
		}
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate reports
	if format == "markdown" || format == "both" {
		mdPath := filepath.Join(outputDir, "failure_analysis.md")
		if verbose {
			fmt.Printf("Generating markdown report: %s\n", mdPath)
		}
		if err := analyzer.GenerateMarkdownReport(report, mdPath); err != nil {
			return fmt.Errorf("failed to generate markdown report: %w", err)
		}

		// Also generate rankings and job summary
		rankingsPath := filepath.Join(outputDir, "test_rankings.md")
		if err := analyzer.GenerateTestRankingsReport(report.TestStats, rankingsPath); err != nil {
			return fmt.Errorf("failed to generate rankings report: %w", err)
		}

		jobSummaryPath := filepath.Join(outputDir, "failures_by_job.md")
		if err := analyzer.GenerateJobSummaryReport(failures, jobSummaryPath); err != nil {
			return fmt.Errorf("failed to generate job summary report: %w", err)
		}
	}

	if format == "json" || format == "both" {
		jsonPath := filepath.Join(outputDir, "failure_analysis.json")
		if verbose {
			fmt.Printf("Generating JSON report: %s\n", jsonPath)
		}
		if err := analyzer.GenerateJSONReport(report, jsonPath); err != nil {
			return fmt.Errorf("failed to generate JSON report: %w", err)
		}
	}

	// Print summary
	fmt.Printf("\nAnalysis complete:\n")
	fmt.Printf("  - Total failures: %d\n", len(failures))
	fmt.Printf("  - Unique failing tests: %d\n", len(report.TestStats))
	fmt.Printf("  - Output directory: %s\n", outputDir)

	if len(report.TestStats) > 0 {
		fmt.Printf("\nTop flaky tests:\n")
		maxShow := 5
		if len(report.TestStats) < maxShow {
			maxShow = len(report.TestStats)
		}
		for i := 0; i < maxShow; i++ {
			s := report.TestStats[i]
			fmt.Printf("  %d. %s (%d failures, %.1f%%)\n",
				i+1, s.TestName, s.TotalFailures, s.FailureRate*100)
		}
	}

	return nil
}

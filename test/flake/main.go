// Package main provides the flake CLI for discovering and analyzing flaky tests.
package main

import (
	"fmt"
	"os"

	"github.com/gruntwork-io/terragrunt/test/flake/cmd"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "flake",
		Usage: "Discover, analyze, and plan resolution of flaky tests",
		Description: `A utility for identifying and analyzing flaky tests in CI pipelines.

This tool uses the GitHub API to discover failed workflow runs on the main branch,
downloads logs and summaries for analysis, and generates reports to help identify
and resolve flaky tests.

Workflow:
  1. Run 'flake discover' to fetch failed CI runs and download logs
  2. Run 'flake analyze' to parse logs and generate failure reports
  3. Review the generated markdown/JSON reports in the analysis/ directory`,
		Commands: []*cli.Command{
			cmd.DiscoverCommand(),
			cmd.AnalyzeCommand(),
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

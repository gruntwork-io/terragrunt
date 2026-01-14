# Flaky Test Discovery & Analysis Utility

A CLI tool for discovering, analyzing, and planning resolution of flaky tests in CI pipelines.

## Overview

This utility helps identify flaky tests by:
1. Fetching failed CI workflow runs from GitHub Actions
2. Downloading and parsing job logs for test failures
3. Analyzing failure patterns and generating reports
4. Supporting resolution planning (for humans and LLMs)

## Installation

```bash
cd test/flake
go build -o flake .
```

## Usage

### 1. Discover Failed Runs

Fetch failed workflow runs and download logs:

```bash
# Basic usage (requires GITHUB_TOKEN env var)
./flake discover

# With options
./flake discover \
  --token $GITHUB_TOKEN \
  --repo gruntwork-io/terragrunt \
  --workflow ci.yml \
  --branch main \
  --limit 20 \
  --verbose
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--token, -t` | GitHub token (or GITHUB_TOKEN env) | Required |
| `--repo, -r` | Repository (owner/repo) | gruntwork-io/terragrunt |
| `--workflow, -w` | Workflow file | ci.yml |
| `--branch, -b` | Branch to check | main |
| `--limit, -n` | Max failed runs to fetch | 20 |
| `--since, -s` | Only runs since date (YYYY-MM-DD) | None |
| `--verbose, -v` | Verbose output | false |

**Output:**
- `logs/` - Downloaded job logs
- `logs/manifest.json` - Discovery metadata
- `summaries/` - Workflow run summaries

### 2. Analyze Failures

Parse logs and generate reports:

```bash
# Basic usage
./flake analyze

# With options
./flake analyze \
  --format both \
  --min-failures 2 \
  --verbose
```

**Options:**
| Flag | Description | Default |
|------|-------------|---------|
| `--input-dir, -i` | Directory with logs | . |
| `--output-dir, -o` | Output directory | analysis |
| `--min-failures` | Min failures to include | 1 |
| `--format, -f` | Output format (markdown/json/both) | both |
| `--verbose, -v` | Verbose output | false |

**Output:**
- `analysis/failure_analysis.md` - Detailed markdown report
- `analysis/failure_analysis.json` - Machine-readable JSON
- `analysis/test_rankings.md` - Tests ranked by failure count
- `analysis/failures_by_job.md` - Failures grouped by CI job

## Workflow

### For Humans

```bash
# 1. Discover recent failures
./flake discover --limit 30 --verbose

# 2. Analyze and generate reports
./flake analyze --min-failures 2

# 3. Review the analysis
cat analysis/failure_analysis.md

# 4. Focus on top flaky tests
cat analysis/test_rankings.md
```

### For LLMs / Automation

```bash
# 1. Discover failures
./flake discover --limit 50

# 2. Generate JSON output
./flake analyze --format json

# 3. Parse JSON for automation
cat analysis/failure_analysis.json | jq '.test_stats[:5]'
```

## Output Formats

### Markdown Report

The markdown report includes:
- Summary statistics (total runs, failures, unique tests)
- Ranked table of flaky tests
- Detailed failure information with log snippets
- Collapsible details for each test

### JSON Report

```json
{
  "generated_at": "2026-01-06T15:30:00Z",
  "total_runs": 20,
  "failed_runs": 8,
  "total_failures": 45,
  "unique_tests": 12,
  "test_stats": [
    {
      "test_name": "TestIntegrationCatalog",
      "total_failures": 6,
      "failure_rate": 0.3,
      "failures": [...],
      "first_seen": "2026-01-01T...",
      "last_seen": "2026-01-05T..."
    }
  ]
}
```

## Directory Structure

```
test/flake/
├── main.go              # CLI entry point
├── go.mod               # Module definition
├── README.md            # This file
├── cmd/                 # Command implementations
├── github/              # GitHub API integration
├── parser/              # Log parsing
├── analyzer/            # Analysis & reporting
├── types/               # Shared types
│
# Runtime directories (gitignored):
├── logs/                # Downloaded logs
├── summaries/           # Workflow summaries
├── analysis/            # Generated reports
└── plan/                # Resolution plans
```

## Planning Resolution

After analysis, create resolution plans in the `plan/` directory:

1. Review `analysis/test_rankings.md` to prioritize
2. Examine log snippets in `analysis/failure_analysis.md`
3. Create plan files like `plan/TestFlakyTest.md` with:
   - Root cause analysis
   - Proposed fix
   - Validation steps
   - Prevention measures

## Requirements

- Go 1.25+
- GitHub token with `repo` and `actions:read` permissions

## Environment Variables

| Variable | Description |
|----------|-------------|
| `GITHUB_TOKEN` | GitHub personal access token |

## Example Session

```bash
$ cd test/flake
$ go build -o flake .

$ ./flake discover --limit 10 --verbose
Fetching failed workflow runs for ci.yml on branch main...
Found 8 failed runs

[1/8] Processing run #1234 (ID: 12345678)
  Found 2 failed jobs
  Downloading logs for job: Test (AWS Tofu)
  Downloading logs for job: Test (Fixtures with OpenTofu)

...

Discovery complete:
  - Runs processed: 8
  - Jobs with logs: 15
  - Logs directory: logs
  - Summaries directory: summaries
  - Manifest: logs/manifest.json

$ ./flake analyze --min-failures 2
Parsing log files...
Found 23 test failures

Analysis complete:
  - Total failures: 23
  - Unique failing tests: 7
  - Output directory: analysis

Top flaky tests:
  1. TestIntegrationCatalog (5 failures, 62.5%)
  2. TestAwsS3Backend (3 failures, 37.5%)
  3. TestSSHClone (3 failures, 37.5%)
```

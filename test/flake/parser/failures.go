package parser

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/test/flake/types"
)

const (
	// Number of lines to capture around a failure for context
	contextLinesBefore = 30
	contextLinesAfter  = 10
)

// ParseLogFile extracts test failures from a log file.
func ParseLogFile(logPath string, runID int64, jobName string, runURL string, failedAt time.Time) ([]types.TestFailure, error) {
	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var failures []types.TestFailure
	var lines []string
	var allLines []string

	scanner := bufio.NewScanner(file)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	// First pass: collect all lines
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Second pass: find failures and extract context
	for lineNum, line := range allLines {
		// Check for failure pattern
		if matches := FailPattern.FindStringSubmatch(line); matches != nil {
			testName := matches[1]

			// Extract context (lines before and after)
			startLine := lineNum - contextLinesBefore
			if startLine < 0 {
				startLine = 0
			}
			endLine := lineNum + contextLinesAfter
			if endLine > len(allLines) {
				endLine = len(allLines)
			}

			snippet := strings.Join(allLines[startLine:endLine], "\n")

			// Try to extract error message
			errorMsg := extractErrorMessage(allLines, lineNum)

			failures = append(failures, types.TestFailure{
				TestName:     testName,
				RunID:        runID,
				JobName:      jobName,
				FailedAt:     failedAt,
				LogSnippet:   cleanLogSnippet(snippet),
				ErrorMessage: errorMsg,
				RunURL:       runURL,
			})
		}
	}

	// Also check for timestamped failures (GitHub Actions format)
	for lineNum, line := range allLines {
		if matches := TimestampedFailPattern.FindStringSubmatch(line); matches != nil {
			testName := matches[1]

			// Check if we already captured this failure
			found := false
			for _, f := range failures {
				if f.TestName == testName {
					found = true
					break
				}
			}
			if found {
				continue
			}

			startLine := lineNum - contextLinesBefore
			if startLine < 0 {
				startLine = 0
			}
			endLine := lineNum + contextLinesAfter
			if endLine > len(allLines) {
				endLine = len(allLines)
			}

			snippet := strings.Join(allLines[startLine:endLine], "\n")
			errorMsg := extractErrorMessage(allLines, lineNum)

			failures = append(failures, types.TestFailure{
				TestName:     testName,
				RunID:        runID,
				JobName:      jobName,
				FailedAt:     failedAt,
				LogSnippet:   cleanLogSnippet(snippet),
				ErrorMessage: errorMsg,
				RunURL:       runURL,
			})
		}
	}

	// Check for panic patterns
	for lineNum, line := range allLines {
		if PanicPattern.MatchString(line) {
			// Try to find which test panicked by looking backwards for === RUN
			testName := "unknown_panic"
			for i := lineNum; i >= 0 && i > lineNum-50; i-- {
				if matches := RunPattern.FindStringSubmatch(allLines[i]); matches != nil {
					testName = matches[1] + "_panic"
					break
				}
			}

			// Check if we already have this failure
			found := false
			for _, f := range failures {
				if f.TestName == testName {
					found = true
					break
				}
			}
			if found {
				continue
			}

			startLine := lineNum - contextLinesBefore
			if startLine < 0 {
				startLine = 0
			}
			endLine := lineNum + contextLinesAfter
			if endLine > len(allLines) {
				endLine = len(allLines)
			}

			snippet := strings.Join(allLines[startLine:endLine], "\n")

			failures = append(failures, types.TestFailure{
				TestName:     testName,
				RunID:        runID,
				JobName:      jobName,
				FailedAt:     failedAt,
				LogSnippet:   cleanLogSnippet(snippet),
				ErrorMessage: line,
				RunURL:       runURL,
			})
		}
	}

	// Deduplicate by test name (keep first occurrence)
	seen := make(map[string]bool)
	var unique []types.TestFailure
	for _, f := range failures {
		if !seen[f.TestName] {
			seen[f.TestName] = true
			unique = append(unique, f)
		}
	}

	_ = lines // silence unused variable warning

	return unique, nil
}

// ParseLogsDir parses all log files in a directory.
func ParseLogsDir(logsDir string, manifest *types.DiscoveryManifest) ([]types.TestFailure, error) {
	var allFailures []types.TestFailure

	// Build a map of run ID to run info
	runMap := make(map[int64]types.WorkflowRun)
	for _, run := range manifest.Runs {
		runMap[run.ID] = run
	}

	// Build a map of job files
	jobMap := make(map[string]types.Job)
	for _, job := range manifest.Jobs {
		if job.LogFile != "" {
			jobMap[job.LogFile] = job
		}
	}

	// Walk the logs directory
	err := filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-log files
		if info.IsDir() || !strings.HasSuffix(path, ".log") {
			return nil
		}

		// Parse run ID from filename
		baseName := filepath.Base(path)
		parts := strings.SplitN(baseName, "_", 2)
		if len(parts) < 2 {
			return nil
		}

		runID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil
		}

		// Get run info
		run, ok := runMap[runID]
		if !ok {
			// Use current time if run not found
			run = types.WorkflowRun{
				ID:        runID,
				CreatedAt: time.Now(),
			}
		}

		// Get job name from filename
		jobName := strings.TrimSuffix(parts[1], ".log")

		// Parse the file
		failures, err := ParseLogFile(path, runID, jobName, run.HTMLURL, run.CreatedAt)
		if err != nil {
			// Log warning but continue
			return nil
		}

		allFailures = append(allFailures, failures...)
		return nil
	})

	return allFailures, err
}

// extractErrorMessage tries to extract a meaningful error message from the context.
func extractErrorMessage(lines []string, failLineNum int) string {
	// Look for error patterns in lines before the failure
	for i := failLineNum - 1; i >= 0 && i > failLineNum-20; i-- {
		line := lines[i]

		// Check for assertion failures
		if matches := AssertionFailPattern.FindStringSubmatch(line); matches != nil {
			return strings.TrimSpace(matches[1])
		}

		// Check for general error patterns
		if matches := ErrorPattern.FindStringSubmatch(line); matches != nil {
			msg := strings.TrimSpace(matches[1])
			if len(msg) > 0 && len(msg) < 200 {
				return msg
			}
		}
	}

	return ""
}

// cleanLogSnippet removes ANSI color codes and excessive whitespace.
func cleanLogSnippet(snippet string) string {
	// Remove ANSI escape codes
	ansiPattern := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	snippet = ansiPattern.ReplaceAllString(snippet, "")

	// Remove GitHub Actions timestamp prefixes
	timestampPattern := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s*`)
	lines := strings.Split(snippet, "\n")
	var cleaned []string
	for _, line := range lines {
		line = timestampPattern.ReplaceAllString(line, "")
		cleaned = append(cleaned, line)
	}

	return strings.Join(cleaned, "\n")
}

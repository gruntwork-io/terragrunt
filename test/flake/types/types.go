// Package types defines the core data structures for flaky test analysis.
package types

import "time"

// WorkflowRun represents a GitHub Actions workflow run.
type WorkflowRun struct {
	ID         int64     `json:"id"`
	RunNumber  int       `json:"run_number"`
	HeadSHA    string    `json:"head_sha"`
	HeadBranch string    `json:"head_branch"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	HTMLURL    string    `json:"html_url"`
}

// Job represents a job within a workflow run.
type Job struct {
	ID         int64  `json:"id"`
	RunID      int64  `json:"run_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	LogFile    string `json:"log_file,omitempty"`
}

// TestFailure represents a single test failure extracted from logs.
type TestFailure struct {
	TestName     string    `json:"test_name"`
	Package      string    `json:"package,omitempty"`
	RunID        int64     `json:"run_id"`
	JobName      string    `json:"job_name"`
	FailedAt     time.Time `json:"failed_at"`
	LogSnippet   string    `json:"log_snippet"`
	ErrorMessage string    `json:"error_message,omitempty"`
	RunURL       string    `json:"run_url,omitempty"`
}

// TestStats represents aggregated statistics for a test.
type TestStats struct {
	TestName      string        `json:"test_name"`
	Package       string        `json:"package,omitempty"`
	TotalFailures int           `json:"total_failures"`
	FailureRate   float64       `json:"failure_rate"`
	Failures      []TestFailure `json:"failures"`
	FirstSeen     time.Time     `json:"first_seen"`
	LastSeen      time.Time     `json:"last_seen"`
}

// AnalysisReport is the complete analysis output.
type AnalysisReport struct {
	GeneratedAt   time.Time            `json:"generated_at"`
	TotalRuns     int                  `json:"total_runs"`
	FailedRuns    int                  `json:"failed_runs"`
	TotalFailures int                  `json:"total_failures"`
	UniqueTests   int                  `json:"unique_tests"`
	TestStats     []TestStats          `json:"test_stats"`
	ByJob         map[string]TestStats `json:"by_job,omitempty"`
}

// DiscoveryManifest tracks what has been discovered.
type DiscoveryManifest struct {
	LastUpdated time.Time     `json:"last_updated"`
	Repository  string        `json:"repository"`
	Branch      string        `json:"branch"`
	Workflow    string        `json:"workflow"`
	Runs        []WorkflowRun `json:"runs"`
	Jobs        []Job         `json:"jobs"`
}

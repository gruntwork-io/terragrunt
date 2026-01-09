// Package parser provides log parsing functionality for Go test output.
package parser

import "regexp"

// Patterns for parsing Go test output.
var (
	// FailPattern matches test failure lines: --- FAIL: TestName (duration)
	FailPattern = regexp.MustCompile(`^.*--- FAIL: (\S+)\s+\(([^)]+)\)`)

	// RunPattern matches test run lines: === RUN TestName
	RunPattern = regexp.MustCompile(`^.*=== RUN\s+(\S+)`)

	// PanicPattern matches panic lines
	PanicPattern = regexp.MustCompile(`^.*panic:`)

	// PackageFailPattern matches package failure lines: FAIL package (duration)
	PackageFailPattern = regexp.MustCompile(`^FAIL\s+(\S+)\s+`)

	// ErrorPattern matches common error patterns
	ErrorPattern = regexp.MustCompile(`(?i)(?:error|Error|ERROR):?\s*(.+)`)

	// TimeoutPattern matches test timeout panics
	TimeoutPattern = regexp.MustCompile(`panic: test timed out after`)

	// GoTestOutputPattern matches go test -v output format
	// Matches lines like: "2024-01-06T10:00:00Z --- FAIL: TestName"
	TimestampedFailPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z\s+--- FAIL: (\S+)`)

	// AssertionFailPattern matches testify assertion failures
	AssertionFailPattern = regexp.MustCompile(`(?:Error Trace|Error:|Messages:)\s*(.+)`)

	// SkipPattern matches skipped tests
	SkipPattern = regexp.MustCompile(`^.*--- SKIP: (\S+)`)
)

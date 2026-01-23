package report

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/invopop/jsonschema"
	"github.com/xeipuuv/gojsonschema"
)

const (
	// csvFieldCount is the expected number of fields in a CSV report row.
	csvFieldCount = 9
	// csvRowOffset accounts for: 0-indexed loop (i starts at 0) + skipped header row.
	csvRowOffset = 2
)

// JSONRun represents a run in JSON format.
type JSONRun struct {
	// Started is the time when the run started.
	Started time.Time `json:"Started" jsonschema:"required"`
	// Ended is the time when the run ended.
	Ended time.Time `json:"Ended" jsonschema:"required"`
	// Reason is the reason for the run result, if any.
	Reason *string `json:"Reason,omitempty" jsonschema:"enum=retry succeeded,enum=error ignored,enum=run error,enum=exclude block,enum=ancestor error"`
	// Cause is the cause of the run result, if any.
	Cause *string `json:"Cause,omitempty"`
	// Name is the name of the run.
	Name string `json:"Name" jsonschema:"required"`
	// Result is the result of the run.
	Result string `json:"Result" jsonschema:"required,enum=succeeded,enum=failed,enum=early exit,enum=excluded"`
	// Ref is the worktree reference (e.g., git commit, branch).
	Ref string `json:"Ref,omitempty"`
	// Cmd is the terraform command (plan, apply, etc.).
	Cmd string `json:"Cmd,omitempty"`
	// Args are the terraform CLI arguments.
	Args []string `json:"Args,omitempty"`
}

// JSONRuns is a slice of JSONRun entries with helper methods.
type JSONRuns []JSONRun

// ParseJSONRuns parses a JSON report from a byte slice.
// Returns a slice of JSONRun entries or an error if parsing fails.
func ParseJSONRuns(data []byte) (JSONRuns, error) {
	var runs JSONRuns
	if err := json.Unmarshal(data, &runs); err != nil {
		return nil, fmt.Errorf("failed to parse JSON report: %w", err)
	}

	return runs, nil
}

// ParseJSONRunsFromFile reads and parses a JSON report from a file.
// Returns a slice of JSONRun entries or an error if reading or parsing fails.
func ParseJSONRunsFromFile(path string) (JSONRuns, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file %s: %w", path, err)
	}

	return ParseJSONRuns(data)
}

// FindByName searches for a run by name.
// Returns the run if found, or nil if not found.
func (runs JSONRuns) FindByName(name string) *JSONRun {
	for i := range runs {
		if runs[i].Name == name {
			return &runs[i]
		}
	}

	return nil
}

// Names returns a slice of all run names.
// Useful for debugging and assertions in tests.
func (runs JSONRuns) Names() []string {
	names := make([]string, len(runs))
	for i := range runs {
		names[i] = runs[i].Name
	}

	return names
}

// CSVRun represents a run parsed from CSV format.
type CSVRun struct {
	Name    string
	Started string
	Ended   string
	Result  string
	Reason  string
	Cause   string
	Ref     string
	Cmd     string
	Args    string
}

// CSVRuns is a slice of CSVRun entries with helper methods.
type CSVRuns []CSVRun

// ParseCSVRuns parses a CSV report from a byte slice.
// Returns a slice of CSVRun entries or an error if parsing fails.
// The first row is expected to be a header row and is skipped.
func ParseCSVRuns(data []byte) (CSVRuns, error) {
	reader := csv.NewReader(bytes.NewReader(data))

	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV report: %w", err)
	}

	// Skip header row
	if len(records) < 1 {
		return CSVRuns{}, nil
	}

	runs := make(CSVRuns, 0, len(records)-1)

	for i, record := range records[1:] {
		if len(record) < csvFieldCount {
			return nil, fmt.Errorf("invalid CSV record at row %d: expected %d fields, got %d", i+csvRowOffset, csvFieldCount, len(record))
		}

		runs = append(runs, CSVRun{
			Name:    record[0],
			Started: record[1],
			Ended:   record[2],
			Result:  record[3],
			Reason:  record[4],
			Cause:   record[5],
			Ref:     record[6],
			Cmd:     record[7],
			Args:    record[8],
		})
	}

	return runs, nil
}

// ParseCSVRunsFromFile reads and parses a CSV report from a file.
// Returns a slice of CSVRun entries or an error if reading or parsing fails.
func ParseCSVRunsFromFile(path string) (CSVRuns, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read report file %s: %w", path, err)
	}

	return ParseCSVRuns(data)
}

// FindByName searches for a run by name.
// Returns the run if found, or nil if not found.
func (runs CSVRuns) FindByName(name string) *CSVRun {
	for i := range runs {
		if runs[i].Name == name {
			return &runs[i]
		}
	}

	return nil
}

// Names returns a slice of all run names.
// Useful for debugging and assertions in tests.
func (runs CSVRuns) Names() []string {
	names := make([]string, len(runs))
	for i := range runs {
		names[i] = runs[i].Name
	}

	return names
}

// SchemaValidationError represents a schema validation error with details.
type SchemaValidationError struct {
	Errors []string
}

func (e *SchemaValidationError) Error() string {
	return fmt.Sprintf("schema validation failed with %d error(s): %v", len(e.Errors), e.Errors)
}

// ValidateJSONReport validates a JSON report against the schema.
// Returns nil if valid, or a SchemaValidationError with details if invalid.
func ValidateJSONReport(data []byte) error {
	schemaBytes, err := json.Marshal(generateReportSchema())
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	schemaLoader := gojsonschema.NewBytesLoader(schemaBytes)
	documentLoader := gojsonschema.NewBytesLoader(data)

	result, err := gojsonschema.Validate(schemaLoader, documentLoader)
	if err != nil {
		return fmt.Errorf("failed to validate report: %w", err)
	}

	if !result.Valid() {
		errors := make([]string, len(result.Errors()))
		for i, validationErr := range result.Errors() {
			errors[i] = validationErr.String()
		}

		return &SchemaValidationError{Errors: errors}
	}

	return nil
}

// ValidateJSONReportFromFile reads and validates a JSON report file against the schema.
// Returns nil if valid, or an error if reading fails or validation fails.
func ValidateJSONReportFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read report file %s: %w", path, err)
	}

	return ValidateJSONReport(data)
}

// WriteToFile writes the report to a file.
func (r *Report) WriteToFile(path string) error {
	tmpFile, err := os.CreateTemp("", "terragrunt-report-*")
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.SortRuns()
	r.mu.Unlock()

	switch r.format {
	case FormatCSV:
		err = r.WriteCSV(tmpFile)
	case FormatJSON:
		err = r.WriteJSON(tmpFile)
	default:
		return fmt.Errorf("unsupported format: %s", r.format)
	}

	if err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close report file: %w", err)
	}

	if r.workingDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(r.workingDir, path)
	}

	return util.MoveFile(tmpFile.Name(), path)
}

// WriteCSV writes the report to a writer in CSV format.
func (r *Report) WriteCSV(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	csvWriter := csv.NewWriter(w)
	defer csvWriter.Flush()

	err := csvWriter.Write([]string{
		"Name",
		"Started",
		"Ended",
		"Result",
		"Reason",
		"Cause",
		"Ref",
		"Cmd",
		"Args",
	})
	if err != nil {
		return err
	}

	for _, run := range r.Runs {
		run.mu.RLock()
		defer run.mu.RUnlock()

		workingDir := effectiveWorkingDir(run, r.workingDir)
		name := nameOfPath(run.Path, workingDir)

		started := run.Started.Format(time.RFC3339)
		ended := run.Ended.Format(time.RFC3339)
		result := string(run.Result)
		reason := ""

		if run.Reason != nil {
			reason = string(*run.Reason)
		}

		cause := ""
		if run.Cause != nil {
			cause = string(*run.Cause)

			if reason == string(ReasonAncestorError) && workingDir != "" {
				cause = strings.TrimPrefix(cause, workingDir+string(os.PathSeparator))
			}
		}

		// Format Args as pipe-separated string for CSV to avoid conflicts with CSV column separator
		args := strings.Join(run.Args, "|")

		err := csvWriter.Write([]string{
			name,
			started,
			ended,
			result,
			reason,
			cause,
			run.Ref,
			run.Cmd,
			args,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// WriteJSON writes the report to a writer in JSON format.
func (r *Report) WriteJSON(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	runs := make([]JSONRun, 0, len(r.Runs))

	for _, run := range r.Runs {
		run.mu.RLock()
		defer run.mu.RUnlock()

		workingDir := effectiveWorkingDir(run, r.workingDir)
		name := nameOfPath(run.Path, workingDir)

		jsonRun := JSONRun{
			Name:    name,
			Started: run.Started,
			Ended:   run.Ended,
			Result:  string(run.Result),
		}

		if run.Reason != nil {
			reason := string(*run.Reason)
			jsonRun.Reason = &reason
		}

		if run.Cause != nil {
			cause := string(*run.Cause)
			if run.Reason != nil && *run.Reason == ReasonAncestorError && workingDir != "" {
				cause = strings.TrimPrefix(cause, workingDir+string(os.PathSeparator))
			}

			jsonRun.Cause = &cause
		}

		if run.Ref != "" {
			jsonRun.Ref = run.Ref
		}

		if run.Cmd != "" {
			jsonRun.Cmd = run.Cmd
		}

		if len(run.Args) > 0 {
			jsonRun.Args = run.Args
		}

		runs = append(runs, jsonRun)
	}

	jsonBytes, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return err
	}

	jsonBytes = append(jsonBytes, '\n')

	_, err = w.Write(jsonBytes)

	return err
}

// WriteSchemaToFile writes a JSON schema for the report to a file.
func (r *Report) WriteSchemaToFile(path string) error {
	tmpFile, err := os.CreateTemp("", "terragrunt-schema-*")
	if err != nil {
		return err
	}

	if err := WriteSchema(tmpFile); err != nil {
		return fmt.Errorf("failed to write schema: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close schema file: %w", err)
	}

	if r.workingDir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(r.workingDir, path)
	}

	return util.MoveFile(tmpFile.Name(), path)
}

// WriteSchema writes a JSON schema for the report to a writer.
func WriteSchema(w io.Writer) error {
	arraySchema := generateReportSchema()

	jsonBytes, err := json.MarshalIndent(arraySchema, "", "  ")
	if err != nil {
		return err
	}

	jsonBytes = append(jsonBytes, '\n')

	_, err = w.Write(jsonBytes)

	return err
}

// nameOfPath returns a name for a path given a working directory.
//
// The logic for determining the name of a given path is:
//
//   - If the path is the same as the working directory, return the base name of the path.
//     This is usually only relevant when performing a `run --all` in a unit directory.
//
//   - If the path is not a subdirectory of the working directory, return the path as is.
//
//   - Otherwise, return the path relative to the working directory, with any leading slashes removed.
func nameOfPath(path string, workingDir string) string {
	// If the path is the same as the working directory,
	// return the base name of the path.
	if path == workingDir {
		return filepath.Base(path)
	}

	// If the path is not a subdirectory of the working directory,
	// return the path as is.
	if !strings.HasPrefix(path, workingDir) {
		return path
	}

	path = strings.TrimPrefix(path, workingDir)
	path = strings.TrimPrefix(path, string(os.PathSeparator))

	return path
}

// effectiveWorkingDir returns the working directory to use for path computation.
// If the run has a DiscoveryWorkingDir set (for worktree scenarios), use that.
// Otherwise, fall back to the report's workingDir.
func effectiveWorkingDir(run *Run, reportWorkingDir string) string {
	if run.DiscoveryWorkingDir != "" {
		return run.DiscoveryWorkingDir
	}

	return reportWorkingDir
}

// generateReportSchema generates the JSON schema for report validation.
func generateReportSchema() *jsonschema.Schema {
	reflector := jsonschema.Reflector{
		DoNotReference: true,
	}

	schema := reflector.Reflect(&JSONRun{})
	schema.Description = "Schema for Terragrunt run report"
	schema.Title = "Terragrunt Run Report Schema"
	schema.ID = "https://terragrunt.gruntwork.io/schemas/run/report/v3/schema.json"

	return &jsonschema.Schema{
		Type:        "array",
		Title:       "Terragrunt Run Report Schema",
		Description: "Array of Terragrunt runs",
		Items:       schema,
	}
}

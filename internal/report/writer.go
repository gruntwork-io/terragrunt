package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/invopop/jsonschema"
)

// JSONRun represents a run in JSON format.
type JSONRun struct {
	// Started is the time when the run started.
	Started time.Time `json:"Started" jsonschema:"required"`
	// Ended is the time when the run ended.
	Ended time.Time `json:"Ended" jsonschema:"required"`
	// Reason is the reason for the run result, if any.
	Reason *string `json:"Reason,omitempty" jsonschema:"enum=retry succeeded,enum=error ignored,enum=run error,enum=--queue-exclude-dir,enum=exclude block,enum=ancestor error"`
	// Cause is the cause of the run result, if any.
	Cause *string `json:"Cause,omitempty"`
	// Name is the name of the run.
	Name string `json:"Name" jsonschema:"required"`
	// Result is the result of the run.
	Result string `json:"Result" jsonschema:"required,enum=succeeded,enum=failed,enum=early exit,enum=excluded"`
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
	})
	if err != nil {
		return err
	}

	for _, run := range r.Runs {
		run.mu.RLock()
		defer run.mu.RUnlock()

		name := nameOfPath(run.Path, r.workingDir)

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

			if reason == string(ReasonAncestorError) && r.workingDir != "" {
				cause = strings.TrimPrefix(cause, r.workingDir+string(os.PathSeparator))
			}
		}

		err := csvWriter.Write([]string{
			name,
			started,
			ended,
			result,
			reason,
			cause,
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

		name := nameOfPath(run.Path, r.workingDir)

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
			if run.Reason != nil && *run.Reason == ReasonAncestorError && r.workingDir != "" {
				cause = strings.TrimPrefix(cause, r.workingDir+string(os.PathSeparator))
			}

			jsonRun.Cause = &cause
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

	if err := r.WriteSchema(tmpFile); err != nil {
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
func (r *Report) WriteSchema(w io.Writer) error {
	reflector := jsonschema.Reflector{
		DoNotReference: true,
	}

	schema := reflector.Reflect(&JSONRun{})

	schema.Description = "Schema for Terragrunt run report"
	schema.Title = "Terragrunt Run Report Schema"
	schema.ID = "https://terragrunt.gruntwork.io/schemas/run/report/v1/schema.json"

	arraySchema := &jsonschema.Schema{
		Type:        "array",
		Title:       "Terragrunt Run Report Schema",
		Description: "Array of Terragrunt runs",
		Items:       schema,
	}

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

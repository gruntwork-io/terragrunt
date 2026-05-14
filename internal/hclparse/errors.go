package hclparse

import "fmt"

// UnexpectedBodyTypeError indicates that an HCL file body was not the expected
// *hclsyntax.Body type. This typically occurs with JSON-format HCL files.
type UnexpectedBodyTypeError struct {
	FilePath string
}

func (e UnexpectedBodyTypeError) Error() string {
	return fmt.Sprintf("unexpected HCL body type in %s (expected native HCL syntax, not JSON)", e.FilePath)
}

// DuplicateUnitNameError indicates that multiple units with the same name were
// found after merging include blocks.
type DuplicateUnitNameError struct {
	Name string
}

func (e DuplicateUnitNameError) Error() string {
	return fmt.Sprintf("duplicate unit name %q after include merge", e.Name)
}

// DuplicateStackNameError indicates that multiple stacks with the same name were
// found after merging include blocks.
type DuplicateStackNameError struct {
	Name string
}

func (e DuplicateStackNameError) Error() string {
	return fmt.Sprintf("duplicate stack name %q after include merge", e.Name)
}

// IncludeValidationError indicates that an included stack file violates
// constraints (e.g. defines locals or nested includes).
type IncludeValidationError struct {
	IncludeName string
	Reason      string
}

func (e IncludeValidationError) Error() string {
	return fmt.Sprintf("included stack file %q: %s", e.IncludeName, e.Reason)
}

// FileReadError indicates a failure to read a file from the filesystem.
type FileReadError struct {
	Err      error
	FilePath string
}

func (e FileReadError) Error() string {
	return fmt.Sprintf("failed to read %s: %s", e.FilePath, e.Err)
}

func (e FileReadError) Unwrap() error {
	return e.Err
}

// FileParseError indicates a failure to parse an HCL file.
type FileParseError struct {
	FilePath string
	Detail   string
}

func (e FileParseError) Error() string {
	return fmt.Sprintf("failed to parse %s: %s", e.FilePath, e.Detail)
}

// FileDecodeError indicates a failure to decode an HCL file into a struct. Err preserves the underlying error (typically hcl.Diagnostics) so callers can extract it via errors.As.
type FileDecodeError struct {
	Err    error
	Name   string
	Detail string
}

func (e FileDecodeError) Error() string {
	return fmt.Sprintf("failed to decode %q: %s", e.Name, e.Detail)
}

func (e FileDecodeError) Unwrap() error {
	return e.Err
}

// FileWriteError indicates a failure to write a file to the filesystem.
type FileWriteError struct {
	Err      error
	FilePath string
}

func (e FileWriteError) Error() string {
	return fmt.Sprintf("failed to write %s: %s", e.FilePath, e.Err)
}

func (e FileWriteError) Unwrap() error {
	return e.Err
}

// DirCreateError indicates a failure to create a directory.
type DirCreateError struct {
	Err     error
	DirPath string
}

func (e DirCreateError) Error() string {
	return fmt.Sprintf("failed to create directory %s: %s", e.DirPath, e.Err)
}

func (e DirCreateError) Unwrap() error {
	return e.Err
}

// LocalEvalError indicates a failure to evaluate a local variable. Err preserves the underlying hcl.Diagnostics so callers can extract them via errors.As.
type LocalEvalError struct {
	Err    error
	Name   string
	Detail string
}

func (e LocalEvalError) Error() string {
	return fmt.Sprintf("failed to evaluate local %q: %s", e.Name, e.Detail)
}

func (e LocalEvalError) Unwrap() error {
	return e.Err
}

// LocalsCycleError indicates that locals have circular dependencies.
type LocalsCycleError struct {
	Names []string
}

func (e LocalsCycleError) Error() string {
	return fmt.Sprintf("could not evaluate locals (possible cycle): %v", e.Names)
}

// LocalsMaxIterError indicates that locals evaluation exceeded the maximum iterations.
type LocalsMaxIterError struct {
	MaxIterations int
	Remaining     int
}

func (e LocalsMaxIterError) Error() string {
	return fmt.Sprintf("locals evaluation exceeded %d iterations with %d unresolved locals", e.MaxIterations, e.Remaining)
}

// MalformedDependencyError indicates a dependency block in an autoinclude file is malformed. Err optionally carries the original HCL diagnostics so callers can extract position info via errors.As/Is.
type MalformedDependencyError struct {
	Err      error
	FilePath string
	Name     string
	Reason   string
}

func (e MalformedDependencyError) Error() string {
	return fmt.Sprintf("malformed dependency %q in %s: %s", e.Name, e.FilePath, e.Reason)
}

func (e MalformedDependencyError) Unwrap() error {
	return e.Err
}

// EmptyArgError indicates that a required string argument was empty.
type EmptyArgError struct {
	Func string
	Arg  string
}

func (e EmptyArgError) Error() string {
	return fmt.Sprintf("hclparse.%s: %s is empty", e.Func, e.Arg)
}

// RefEvalError indicates that a unit's or stack's lazy `path` or `source` expression could not be evaluated. Surfaced on the bootstrap parse path (i.e. when the caller did not supply pre-resolved UnitRefs/StackRefs from the production parser), where evaluation runs against a minimal stdlib-only eval context.
type RefEvalError struct {
	Err  error
	Kind string
	Name string
	Attr string
}

func (e RefEvalError) Error() string {
	return fmt.Sprintf("evaluate %s %q %s: %s", e.Kind, e.Name, e.Attr, e.Err)
}

func (e RefEvalError) Unwrap() error {
	return e.Err
}

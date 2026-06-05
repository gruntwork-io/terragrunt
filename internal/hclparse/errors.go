package hclparse

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
)

// unknownPlaceholder is the fallback shown in error messages when an optional name field is empty.
const unknownPlaceholder = "(unknown)"

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
// constraints (e.g. defines locals or nested includes). Err preserves the
// underlying error (such as hcl.Diagnostics from include-path evaluation)
// so callers can extract it via errors.As.
type IncludeValidationError struct {
	Err         error
	IncludeName string
	Reason      string
}

func (e IncludeValidationError) Error() string {
	return fmt.Sprintf("included stack file %q: %s", e.IncludeName, e.Reason)
}

func (e IncludeValidationError) Unwrap() error {
	return e.Err
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

// FileParseError indicates a failure to parse an HCL file; Err preserves the underlying diagnostics.
type FileParseError struct {
	Err      error
	FilePath string
}

func (e FileParseError) Error() string {
	if e.Err == nil {
		return "failed to parse " + e.FilePath
	}

	return fmt.Sprintf("failed to parse %s: %s", e.FilePath, e.Err)
}

func (e FileParseError) Unwrap() error {
	return e.Err
}

// FileDecodeError indicates a failure to decode an HCL file into a struct; Err preserves the underlying diagnostics.
type FileDecodeError struct {
	Err  error
	Name string
}

func (e FileDecodeError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("failed to decode %q", e.Name)
	}

	return fmt.Sprintf("failed to decode %q: %s", e.Name, e.Err)
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

// LocalEvalError indicates a failure to evaluate a local variable; Err preserves the underlying diagnostics.
type LocalEvalError struct {
	Err  error
	Name string
}

func (e LocalEvalError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("failed to evaluate local %q", e.Name)
	}

	return fmt.Sprintf("failed to evaluate local %q: %s", e.Name, e.Err)
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

// StackAutoIncludeDependencyValuesError indicates that a stack-level autoinclude
// injects a unit or stack whose values reference dependency outputs. Dependency
// outputs are not available at stack generate time (they resolve at unit run time),
// so this pattern cannot be generated.
type StackAutoIncludeDependencyValuesError struct {
	Subject   *hcl.Range
	StackName string
	UnitName  string
}

func (e StackAutoIncludeDependencyValuesError) Error() string {
	stack := e.StackName
	if stack == "" {
		stack = unknownPlaceholder
	}

	target := e.UnitName
	if target == "" {
		target = unknownPlaceholder
	}

	return fmt.Sprintf(
		"stack %q autoinclude injects unit/stack %q whose values reference dependency outputs, "+
			"which are not available at stack generate time. "+
			"Use the supported cross-level pattern instead: "+
			"pass only unit.X.path through values on the child stack block, and declare the dependency inside the nested unit's own autoinclude so it resolves at the unit run.",
		stack, target,
	)
}

// AutoIncludeValuesReferenceError indicates an autoinclude expression references the unit-scoped values namespace, which is unavailable at stack generate time.
type AutoIncludeValuesReferenceError struct {
	Subject   *hcl.Range
	Kind      string
	Component string
	Attr      string
}

func (e AutoIncludeValuesReferenceError) Error() string {
	component := e.Component
	if component == "" {
		component = unknownPlaceholder
	}

	attr := e.Attr
	if attr == "" {
		attr = unknownPlaceholder
	}

	return fmt.Sprintf(
		"autoinclude for %s %q references values.* in %q, but values is a unit-scoped namespace that is not available when the stack file is generated; "+
			"replace the reference with a stack-level local declared in terragrunt.stack.hcl or a literal value",
		e.Kind, component, attr,
	)
}

// AutoIncludeNestedError indicates an autoinclude block is nested inside another autoinclude block, which is disallowed.
type AutoIncludeNestedError struct {
	Subject   *hcl.Range
	Kind      string
	Component string
}

func (e AutoIncludeNestedError) Error() string {
	component := e.Component
	if component == "" {
		component = unknownPlaceholder
	}

	return fmt.Sprintf(
		"autoinclude for %s %q nests an autoinclude block, which is not allowed; "+
			"an autoinclude block must not contain another autoinclude block",
		e.Kind, component,
	)
}

// AutoIncludeLocalsBlockError indicates an autoinclude body defines a locals block, which is disallowed in favor of stack-level locals.
type AutoIncludeLocalsBlockError struct {
	Subject   *hcl.Range
	Kind      string
	Component string
}

func (e AutoIncludeLocalsBlockError) Error() string {
	component := e.Component
	if component == "" {
		component = unknownPlaceholder
	}

	return fmt.Sprintf(
		"autoinclude for %s %q defines a locals block, which is not allowed; "+
			"declare locals at the stack level in terragrunt.stack.hcl so they resolve uniformly at generate time",
		e.Kind, component,
	)
}

// EmptyArgError indicates that a required string argument was empty.
type EmptyArgError struct {
	Func string
	Arg  string
}

func (e EmptyArgError) Error() string {
	return fmt.Sprintf("hclparse.%s: %s is empty", e.Func, e.Arg)
}

// PartialEvalDepthExceededError indicates that PartialEval hit its recursion guard.
type PartialEvalDepthExceededError struct {
	MaxDepth int
}

func (e PartialEvalDepthExceededError) Error() string {
	return fmt.Sprintf("partial evaluation exceeded maximum recursion depth %d", e.MaxDepth)
}

// BlockDepthExceededError indicates that nested-block traversal of an autoinclude body hit its recursion guard.
type BlockDepthExceededError struct {
	MaxDepth int
}

func (e BlockDepthExceededError) Error() string {
	return fmt.Sprintf("autoinclude block nesting exceeded maximum recursion depth %d", e.MaxDepth)
}

// StackRecursionDepthExceededError indicates that nested-stack unit-path expansion hit its recursion guard.
type StackRecursionDepthExceededError struct {
	StackDir string
	MaxDepth int
}

func (e StackRecursionDepthExceededError) Error() string {
	return fmt.Sprintf("nested stack expansion exceeded maximum recursion depth %d at %q", e.MaxDepth, e.StackDir)
}

// PartialEvalUnresolvedError indicates that partial evaluation could not produce a final cty value.
type PartialEvalUnresolvedError struct {
	Err    error
	Reason string
}

func (e PartialEvalUnresolvedError) Error() string {
	msg := "partial evaluation could not resolve expression"
	if e.Reason != "" {
		msg += ": " + e.Reason
	}

	if e.Err == nil {
		return msg
	}

	return fmt.Sprintf("%s: %s", msg, e.Err)
}

func (e PartialEvalUnresolvedError) Unwrap() error {
	return e.Err
}

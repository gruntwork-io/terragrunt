// Package writer provides writer types for Terragrunt I/O.
package writer

import "io"

// Writers groups the writer-related fields that travel together across
// TerragruntOptions, ParsingContext, shell.RunOptions and engine.ExecutionOptions.
type Writers struct {
	// Writer is the primary output writer (defaults to os.Stdout).
	Writer io.Writer
	// ErrWriter is the error output writer (defaults to os.Stderr).
	ErrWriter io.Writer
	// LogShowAbsPaths disables replacing full paths in logs with short relative paths.
	LogShowAbsPaths bool
	// LogDisableErrorSummary is a flag to skip the error summary.
	LogDisableErrorSummary bool
}

// writerUnwrapper is any writer that can provide its underlying parent writer.
// This interface allows extracting the original writer from wrapped writers.
type writerUnwrapper interface {
	Unwrap() io.Writer
}

// OriginalWriter wraps an io.Writer and implements writerUnwrapper to preserve
// access to the original writer even after it's been wrapped by other writers.
// This is used to maintain access to the original stdout/stderr writers after they
// are wrapped by log writers in logTFOutput.
type OriginalWriter struct {
	w io.Writer
}

// NewOriginalWriter creates a new OriginalWriter that wraps the given writer.
func NewOriginalWriter(w io.Writer) *OriginalWriter {
	return &OriginalWriter{w: w}
}

// Write implements io.Writer by delegating to the wrapped writer.
func (ow *OriginalWriter) Write(p []byte) (int, error) {
	return ow.w.Write(p)
}

// Unwrap implements writerUnwrapper by returning the wrapped writer.
func (ow *OriginalWriter) Unwrap() io.Writer {
	return ow.w
}

// WrappedWriter wraps an io.Writer and implements writerUnwrapper to preserve
// access to an underlying original writer. This is used to wrap the result of
// buildOutWriter/buildErrWriter so the original writer can still be extracted.
type WrappedWriter struct {
	wrapped  io.Writer
	original io.Writer
}

// NewWrappedWriter creates a new WrappedWriter that wraps the given writer
// and preserves access to the original writer.
func NewWrappedWriter(wrapped, original io.Writer) *WrappedWriter {
	return &WrappedWriter{
		wrapped:  wrapped,
		original: original,
	}
}

// Write implements io.Writer by delegating to the wrapped writer.
func (ww *WrappedWriter) Write(p []byte) (int, error) {
	return ww.wrapped.Write(p)
}

// Unwrap implements writerUnwrapper by returning the original writer.
func (ww *WrappedWriter) Unwrap() io.Writer {
	return ww.original
}

// ExtractOriginalWriter extracts the original writer from a potentially wrapped writer.
// If the writer implements writerUnwrapper, it recursively extracts the parent.
// Otherwise, it returns the writer as-is.
func ExtractOriginalWriter(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}

	if u, ok := w.(writerUnwrapper); ok {
		parent := u.Unwrap()

		return ExtractOriginalWriter(parent)
	}

	return w
}

// Package writer provides writer types for Terragrunt I/O.
package writer

import (
	"io"
	"sync"
)

// Writers groups the stdout/stderr handles that travel together across
// TerragruntOptions, ParsingContext, shell.ShellOptions and
// engine.ExecutionOptions. The log-formatting flags that once rode along
// here (LogShowAbsPaths, LogDisableErrorSummary) now live directly on those
// structs.
type Writers struct {
	// Writer is the primary output writer (defaults to os.Stdout).
	Writer io.Writer
	// ErrWriter is the error output writer (defaults to os.Stderr).
	ErrWriter io.Writer
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

// SyncWriter wraps an io.Writer and serializes concurrent Write calls using a mutex.
// This prevents data races when the same underlying writer is shared between goroutines
// that each hold an independent logrus.Logger mutex.
type SyncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// NewSyncWriter returns a SyncWriter that guards w with a mutex.
func NewSyncWriter(w io.Writer) *SyncWriter {
	return &SyncWriter{w: w}
}

// Write implements io.Writer; concurrent calls are serialized by mu.
func (sw *SyncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// Unwrap implements writerUnwrapper so ExtractOriginalWriter can traverse the chain.
func (sw *SyncWriter) Unwrap() io.Writer {
	return sw.w
}

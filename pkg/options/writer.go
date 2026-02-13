package options

import "io"

// parentWriterProvider is any writer that can provide its underlying parent writer.
// This interface allows extracting the original writer from wrapped writers.
type parentWriterProvider interface {
	ParentWriter() io.Writer
}

// OriginalWriter wraps an io.Writer and implements parentWriterProvider to preserve
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

// ParentWriter implements parentWriterProvider by returning the wrapped writer.
func (ow *OriginalWriter) ParentWriter() io.Writer {
	return ow.w
}

// WrappedWriter wraps an io.Writer and implements parentWriterProvider to preserve
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

// ParentWriter implements parentWriterProvider by returning the original writer.
func (ww *WrappedWriter) ParentWriter() io.Writer {
	return ww.original
}

// ExtractOriginalWriter extracts the original writer from a potentially wrapped writer.
// If the writer implements parentWriterProvider, it recursively extracts the parent.
// Otherwise, it returns the writer as-is.
func ExtractOriginalWriter(w io.Writer) io.Writer {
	if w == nil {
		return nil
	}

	if pwp, ok := w.(parentWriterProvider); ok {
		parent := pwp.ParentWriter()

		return ExtractOriginalWriter(parent)
	}

	return w
}

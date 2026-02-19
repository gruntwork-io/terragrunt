package options

import (
	"io"

	"github.com/gruntwork-io/terragrunt/internal/writerutil"
)

// NewOriginalWriter creates a new OriginalWriter that wraps the given writer.
//
// Deprecated: Use writerutil.NewOriginalWriter instead.
func NewOriginalWriter(w io.Writer) *writerutil.OriginalWriter {
	return writerutil.NewOriginalWriter(w)
}

// NewWrappedWriter creates a new WrappedWriter that wraps the given writer
// and preserves access to the original writer.
//
// Deprecated: Use writerutil.NewWrappedWriter instead.
func NewWrappedWriter(wrapped, original io.Writer) *writerutil.WrappedWriter {
	return writerutil.NewWrappedWriter(wrapped, original)
}

// ExtractOriginalWriter extracts the original writer from a potentially wrapped writer.
//
// Deprecated: Use writerutil.ExtractOriginalWriter instead.
func ExtractOriginalWriter(w io.Writer) io.Writer {
	return writerutil.ExtractOriginalWriter(w)
}

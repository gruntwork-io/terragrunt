// Package format renders discovered catalog components in non-interactive
// output formats. Renderers write each component as it arrives rather than
// collecting them, so output reaches the consumer while discovery is still
// running.
package format

import (
	"errors"
	"fmt"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

const (
	// JSONL names the JSON Lines format: one JSON object per line.
	JSONL = "jsonl"

	// MD names the Markdown format: one document, with a section per component.
	MD = "md"
)

// ErrUnsupportedFormat is returned by [NewRenderer] for a format name that
// has no renderer.
var ErrUnsupportedFormat = errors.New("unsupported catalog output format")

// Summary describes what a finished render covered: how many components were
// written, and how many distinct sources they came from.
type Summary struct {
	Entries int
	Sources int
}

// Renderer writes catalog components to w. Open runs once before the first
// component and Close once after the last, so document-oriented formats can
// wrap the stream in a header and a footer.
type Renderer interface {
	Open(w io.Writer) error
	Entry(w io.Writer, e *tui.ComponentEntry) error
	Close(w io.Writer, summary Summary) error
}

// NewRenderer returns the renderer for the named format.
func NewRenderer(name string) (Renderer, error) {
	switch name {
	case JSONL:
		return NewJSONLRenderer(), nil
	case MD:
		return NewMarkdownRenderer(), nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedFormat, name)
	}
}

// flush pushes a buffering writer's contents through to the consumer, so a
// reader on the other end of a pipe sees each component when it is rendered
// rather than when the buffer happens to fill.
func flush(w io.Writer) error {
	f, ok := w.(interface{ Flush() error })
	if !ok {
		return nil
	}

	return f.Flush()
}

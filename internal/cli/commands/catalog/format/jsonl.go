package format

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

// JSONLRenderer writes each component as a JSON object on its own line.
type JSONLRenderer struct{}

// NewJSONLRenderer returns a renderer that emits JSON Lines.
func NewJSONLRenderer() *JSONLRenderer {
	return &JSONLRenderer{}
}

// Open implements [Renderer]. JSON Lines has no document header.
func (r *JSONLRenderer) Open(_ io.Writer) error {
	return nil
}

// Entry implements [Renderer], writing e as one line.
func (r *JSONLRenderer) Entry(w io.Writer, e *tui.ComponentEntry) error {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)

	// README bodies routinely contain angle brackets and ampersands, and
	// escaping those would leave the documentation unreadable to anything
	// that prints a record's `doc` field.
	enc.SetEscapeHTML(false)

	if err := enc.Encode(NewEntry(e)); err != nil {
		return err
	}

	// Encode fully before writing, so a record reaches the stream in one Write
	// rather than in fragments an interleaved writer could split. That is not
	// atomicity: a README of a few megabytes exceeds PIPE_BUF, and os.File
	// splits it across several write syscalls. What keeps records whole is
	// that a single goroutine drains the component channel, so nothing else
	// writes to this stream. Merging stderr into it (2>&1) forfeits that.
	if _, err := w.Write(buf.Bytes()); err != nil {
		return err
	}

	return flush(w)
}

// Close implements [Renderer]. JSON Lines has no document footer.
func (r *JSONLRenderer) Close(_ io.Writer, _ Summary) error {
	return nil
}

package catalog

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// FormatTUI browses the catalog through the interactive terminal user
	// interface. It is the only format that needs a terminal.
	FormatTUI = "tui"

	// FormatJSONL writes each discovered component to standard output as a
	// JSON object on its own line.
	FormatJSONL = format.JSONL

	// FormatMD writes the discovered components to standard output as a
	// Markdown document, with a section per component.
	FormatMD = format.MD
)

// Options holds the settings of a single `terragrunt catalog` invocation.
type Options struct {
	*options.TerragruntOptions

	Format string
}

// NewOptions returns catalog options with no format requested, leaving the
// command to fill one in from [DefaultFormat].
func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
	}
}

// DefaultFormat returns the format a run uses when none is requested:
// [FormatTUI] when stdin and stdout are both terminals, since the TUI reads
// keys from one and draws on the other, and [FormatJSONL] otherwise, so piping
// the command yields entries a program can parse.
func DefaultFormat(t *venv.Terminal) string {
	if t.StdinIsTTY() && t.StdoutIsTTY() {
		return FormatTUI
	}

	return FormatJSONL
}

// Validate reports whether the requested settings can be acted on.
func (o *Options) Validate() error {
	return o.validateFormat()
}

func (o *Options) validateFormat() error {
	switch o.Format {
	case FormatTUI:
		return nil
	case FormatJSONL:
		return nil
	case FormatMD:
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

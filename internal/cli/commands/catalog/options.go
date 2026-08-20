package catalog

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/format"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// FormatTUI browses the catalog through the interactive terminal user
	// interface. It is the default, and the only format that needs a terminal.
	FormatTUI = "tui"

	// FormatJSONL writes each discovered component to standard output as a
	// JSON object on its own line.
	FormatJSONL = format.JSONL

	// FormatMD writes the discovered components to standard output as a
	// Markdown document, with a section per component.
	FormatMD = format.MD
)

// ErrFormatRequiresExperiment is returned when a non-interactive format is
// requested without the 'catalog-format' experiment.
var ErrFormatRequiresExperiment = errors.New(
	"non-interactive catalog formats require usage of the 'catalog-format' experiment" +
		" (e.g., --experiment=catalog-format)",
)

// Options holds the settings of a single `terragrunt catalog` invocation.
type Options struct {
	*options.TerragruntOptions

	Format string
}

// NewOptions returns catalog options defaulting to the terminal user interface.
func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		Format:            FormatTUI,
	}
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

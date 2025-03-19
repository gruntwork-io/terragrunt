package find

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// FormatText outputs the discovered configurations in text format.
	FormatText = "text"

	// FormatJSON outputs the discovered configurations in JSON format.
	FormatJSON = "json"

	// SortAlpha sorts the discovered configurations in alphabetical order.
	SortAlpha = "alpha"

	// SortDAG sorts the discovered configurations in a topological sort order.
	SortDAG = "dag"

	// ModeNormal is the default mode for the find command.
	ModeNormal = "normal"

	// ModeDAG is the mode for the find command that sorts and groups output in DAG order.
	ModeDAG = "dag"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// JSON determines whether to output in JSON format.
	// Alias for --format=json.
	JSON bool

	// DAG determines whether to sort output in DAG order.
	// Alias for --sort=dag.
	DAG bool

	// Mode determines the mode of the find command.
	Mode string

	// Hidden determines whether to detect hidden directories.
	Hidden bool

	// Dependencies determines whether to include dependencies in the output.
	Dependencies bool

	// External determines whether to include external dependencies in the output.
	External bool
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		Format:            FormatText,
		Mode:              ModeNormal,
		Hidden:            false,
	}
}

func (o *Options) Validate() error {
	errs := []error{}

	if err := o.validateFormat(); err != nil {
		errs = append(errs, err)
	}

	if err := o.validateMode(); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return errors.New(errors.Join(errs...))
	}

	return nil
}

func (o *Options) validateFormat() error {
	switch o.Format {
	case FormatText:
		return nil
	case FormatJSON:
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

func (o *Options) validateMode() error {
	switch o.Mode {
	case ModeNormal:
		return nil
	case ModeDAG:
		return nil
	default:
		return errors.New("invalid mode: " + o.Mode)
	}
}

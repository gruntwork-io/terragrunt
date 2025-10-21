package find

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// FormatText outputs the discovered components in text format.
	FormatText = "text"

	// FormatJSON outputs the discovered components in JSON format.
	FormatJSON = "json"

	// SortAlpha sorts the discovered components in alphabetical order.
	SortAlpha = "alpha"

	// SortDAG sorts the discovered components in a topological sort order.
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

	// Mode determines the mode of the find command.
	Mode string

	// QueueConstructAs constructs the queue as if a particular command was run.
	QueueConstructAs string

	// JSON determines if the output should be in JSON format.
	// Alias for --format=json.
	JSON bool

	// DAG determines if the output should be in DAG mode.
	DAG bool

	// Hidden determines if hidden components should be included in the output.
	Hidden bool

	// Dependencies determines if dependencies should be included in the output.
	Dependencies bool

	// Exclude determines if exclude components should be included in the output.
	Exclude bool

	// Include determines if Include components should be included in the output.
	Include bool

	// External determines if external dependencies should be included in the output.
	External bool

	// Reading determines if the list of files that are read by components should be included in the output.
	Reading bool
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

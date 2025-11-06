package list

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// FormatText outputs the discovered configurations in text format.
	FormatText = "text"

	// FormatTree outputs the discovered configurations in tree format.
	FormatTree = "tree"

	// FormatLong outputs the discovered configurations in long format.
	FormatLong = "long"

	// FormatDot outputs the discovered configurations in GraphViz DOT format.
	FormatDot = "dot"

	// SortAlpha sorts the discovered configurations in alphabetical order.
	SortAlpha = "alpha"

	// SortDAG sorts the discovered configurations in a topological sort order.
	SortDAG = "dag"

	// ModeNormal is the default mode for the list command.
	ModeNormal = "normal"

	// ModeDAG is the mode for the list command that sorts and groups output in DAG order.
	ModeDAG = "dag"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// Mode determines the mode of the list command.
	Mode string

	// QueueConstructAs constructs the queue as if a particular command was run.
	QueueConstructAs string

	// Hidden determines whether to detect hidden directories.
	Hidden bool

	// Dependencies determines whether to include dependencies in the output.
	Dependencies bool

	// External determines whether to include external dependencies in the output.
	External bool

	// Tree determines whether to output in tree format.
	Tree bool

	// Long determines whether the output should be in long format.
	Long bool

	// DAG determines whether to output in DAG format.
	DAG bool
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
	case FormatTree:
		return nil
	case FormatLong:
		return nil
	case FormatDot:
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

func (o *Options) validateMode() error {
	switch o.Mode {
	case ModeNormal:
		return nil
	case SortDAG:
		return nil
	default:
		return errors.New("invalid mode: " + o.Mode)
	}
}

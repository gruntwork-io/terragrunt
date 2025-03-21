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
	Format       string
	Mode         string
	JSON         bool
	DAG          bool
	Hidden       bool
	Dependencies bool
	External     bool
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

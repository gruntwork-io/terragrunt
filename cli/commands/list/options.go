package list

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// FormatText outputs the discovered configurations in text format.
	FormatText = "text"

	// FormatJSON outputs the discovered configurations in JSON format.
	FormatJSON = "json"

	// FormatTree outputs the discovered configurations in tree format.
	FormatTree = "tree"

	// FormatLong outputs the discovered configurations in long format.
	FormatLong = "long"

	// SortAlpha sorts the discovered configurations in alphabetical order.
	SortAlpha = "alpha"

	// SortDAG sorts the discovered configurations in a topological sort order.
	SortDAG = "dag"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// JSON determines whether to output in JSON format.
	// Alias for --format=json.
	JSON bool

	// Sort determines the sort order of the output.
	Sort string

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
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		Format:            FormatText,
		Sort:              SortAlpha,
		Hidden:            false,
	}
}

func (o *Options) Validate() error {
	errs := []error{}

	if err := o.validateFormat(); err != nil {
		errs = append(errs, err)
	}

	if err := o.validateSort(); err != nil {
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
	case FormatTree:
		return nil
	case FormatLong:
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

func (o *Options) validateSort() error {
	switch o.Sort {
	case SortAlpha:
		return nil
	case SortDAG:
		return nil
	default:
		return errors.New("invalid sort: " + o.Sort)
	}
}

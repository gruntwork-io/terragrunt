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

	// SortAlpha sorts the discovered configurations in alphabetical order.
	SortAlpha = "alpha"

	// SortDAG sorts the discovered configurations in a topological sort order.
	SortDAG = "dag"

	// GroupByFS groups the discovered configurations by filesystem structure.
	GroupByFS = "fs"

	// GroupByDAG groups the discovered configurations by DAG relationships.
	GroupByDAG = "dag"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// Sort determines the sort order of the output.
	Sort string

	// GroupBy determines how to group the configurations in the output.
	GroupBy string

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
		Sort:              SortAlpha,
		GroupBy:           GroupByFS,
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

	if err := o.validateGroupBy(); err != nil {
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

func (o *Options) validateGroupBy() error {
	switch o.GroupBy {
	case GroupByFS:
		return nil
	case GroupByDAG:
		return nil
	default:
		return errors.New("invalid group-by: " + o.GroupBy)
	}
}

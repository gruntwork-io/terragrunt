package find

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// Sort determines the sort order of the output.
	Sort string
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		Format:            "text",
		Sort:              "alpha",
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
	case "text":
		return nil
	case "json":
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

func (o *Options) validateSort() error {
	switch o.Sort {
	case "alpha":
		return nil
	default:
		return errors.New("invalid sort: " + o.Sort)
	}
}

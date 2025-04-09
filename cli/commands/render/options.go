package render

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// FormatJSON outputs the config in JSON format.
	FormatJSON = "json"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// Write the rendered config to a file.
	Write bool
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		Format:            FormatJSON,
		Write:             false,
	}
}

func (o *Options) WithWrite() *Options {
	o.Write = true
	return o
}

func (o *Options) WithFormat(format string) *Options {
	o.Format = format
	return o
}

func (o *Options) Validate() error {
	if err := o.validateFormat(); err != nil {
		return err
	}

	return nil
}

func (o *Options) validateFormat() error {
	switch o.Format {
	case FormatJSON:
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

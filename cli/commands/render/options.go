package render

import (
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
)

const (
	// FormatHCL outputs the config in HCL format.
	FormatHCL = "hcl"

	// FormatJSON outputs the config in JSON format.
	FormatJSON = "json"
)

type Options struct {
	*options.TerragruntOptions

	// Format determines the format of the output.
	Format string

	// OutputPath is the path to the file to write the rendered config to.
	// This configuration is relative to the Terragrunt config path.
	OutputPath string

	// Write the rendered config to a file.
	Write bool

	// RenderMetadata adds metadata to the rendered config.
	RenderMetadata bool

	// DisableDependentModules disables the identification of dependent modules when rendering config.
	DisableDependentModules bool
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions:       opts,
		Format:                  FormatHCL,
		Write:                   false,
		RenderMetadata:          false,
		DisableDependentModules: false,
	}
}

func (o *Options) Clone() *Options {
	return &Options{
		TerragruntOptions:       o.TerragruntOptions.Clone(),
		Format:                  o.Format,
		OutputPath:              o.OutputPath,
		Write:                   o.Write,
		RenderMetadata:          o.RenderMetadata,
		DisableDependentModules: o.DisableDependentModules,
	}
}

func (o *Options) Validate() error {
	if err := o.validateFormat(); err != nil {
		return err
	}

	return nil
}

func (o *Options) validateFormat() error {
	switch o.Format {
	case FormatHCL:
		return nil
	case FormatJSON:
		return nil
	default:
		return errors.New("invalid format: " + o.Format)
	}
}

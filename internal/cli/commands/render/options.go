package render

import (
	"errors"

	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const (
	// FormatHCL outputs the config in HCL format.
	FormatHCL = "hcl"

	// FormatJSON outputs the config in JSON format.
	FormatJSON = "json"

	hclOutputName  = "terragrunt.rendered.hcl"
	jsonOutputName = "terragrunt.rendered.json"
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
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
		Format:            FormatHCL,
		Write:             false,
		RenderMetadata:    false,
	}
}

func (o *Options) Clone() *Options {
	return &Options{
		TerragruntOptions: o.TerragruntOptions.Clone(),
		Format:            o.Format,
		OutputPath:        o.OutputPath,
		Write:             o.Write,
		RenderMetadata:    o.RenderMetadata,
	}
}

// Validate rejects an unrecognized format. It also fills in [Options.OutputPath] with a
// name derived from the format when a write is requested without one.
func (o *Options) Validate() error {
	if err := validateFormat(o.Format); err != nil {
		return err
	}

	if o.Write && o.OutputPath == "" {
		o.OutputPath = defaultOutputName(o.Format)
	}

	return nil
}

func validateFormat(format string) error {
	switch format {
	case FormatHCL, FormatJSON:
		return nil
	default:
		return errors.New("invalid format: " + format)
	}
}

func defaultOutputName(format string) string {
	if format == FormatJSON {
		return jsonOutputName
	}

	return hclOutputName
}

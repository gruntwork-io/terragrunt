package browse

import (
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Options holds the browse command's configuration.
type Options struct {
	*options.TerragruntOptions
}

// NewOptions returns Options wrapping the given Terragrunt options.
func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
	}
}

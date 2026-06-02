package browse

import (
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

type Options struct {
	*options.TerragruntOptions

	// NoHidden determines if hidden directories should be excluded from discovery.
	NoHidden bool
}

func NewOptions(opts *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: opts,
	}
}

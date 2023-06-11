package validateinputs

import "github.com/gruntwork-io/terragrunt/options"

type Options struct {
	*options.TerragruntOptions

	// ValidateStrict mode
	ValidateStrict bool
}

func NewOptions(global *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: global,
	}
}

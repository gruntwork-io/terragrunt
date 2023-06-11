package hclfmt

import "github.com/gruntwork-io/terragrunt/options"

type Options struct {
	*options.TerragruntOptions

	// The file which hclfmt should be specifically run on
	HclFile string
}

func NewOptions(global *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: global,
	}
}

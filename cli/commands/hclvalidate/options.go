package hclvalidate

import "github.com/gruntwork-io/terragrunt/options"

type Options struct {
	*options.TerragruntOptions

	ShowInvalidConfigPath bool
	JSONOutput            bool
}

func NewOptions(general *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: general,
	}
}

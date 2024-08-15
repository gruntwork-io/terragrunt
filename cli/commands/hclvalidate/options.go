package hclvalidate

import "github.com/gruntwork-io/terragrunt/options"

// Options is the struct that holds the options for the hclvalidate command.
type Options struct {
	*options.TerragruntOptions

	ShowConfigPath bool
	JSONOutput     bool
}

// NewOptions returns a new Options struct with the given TerragruntOptions.
func NewOptions(general *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: general,
	}
}

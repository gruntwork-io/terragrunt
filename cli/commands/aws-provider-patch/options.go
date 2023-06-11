package awsproviderpatch

import "github.com/gruntwork-io/terragrunt/options"

type Options struct {
	*options.TerragruntOptions

	// Attributes to override in AWS provider nested within modules as part of the aws-provider-patch command. See that command for more info.
	AwsProviderPatchOverrides map[string]string
}

func NewOptions(global *options.TerragruntOptions) *Options {
	return &Options{
		TerragruntOptions: global,
	}
}

// Package awsproviderpatch provides the `aws-provider-patch` command.
//
// The `aws-provider-patch` command finds all Terraform modules nested in the current code (i.e., in the .terraform/modules
// folder), looks for provider "aws" { ... } blocks in those modules, and overwrites the attributes in those provider
// blocks with the attributes specified in terragrntOptions.
//
// For example, if were running Terragrunt against code that contained a module:
//
//	module "example" {
//	  source = "<URL>"
//	}
//
// When you run 'init', Terraform would download the code for that module into .terraform/modules. This function would
// scan that module code for provider blocks:
//
//	provider "aws" {
//	   region = var.aws_region
//	}
//
// And if AwsProviderPatchOverrides in opts was set to map[string]string{"region": "us-east-1"}, then this
// method would update the module code to:
//
//	provider "aws" {
//	   region = "us-east-1"
//	}
//
// This is a temporary workaround for a Terraform bug (https://github.com/hashicorp/terraform/issues/13018) where
// any dynamic values in nested provider blocks are not handled correctly when you call 'terraform import', so by
// temporarily hard-coding them, we can allow 'import' to work.
package awsproviderpatch

import (
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/cli"
)

const (
	CommandName = "aws-provider-patch"

	FlagNameTerragruntOverrideAttr = "terragrunt-override-attr"
)

func NewFlags(opts *options.TerragruntOptions) cli.Flags {
	return cli.Flags{
		&cli.MapFlag[string, string]{
			Name:        FlagNameTerragruntOverrideAttr,
			Destination: &opts.AwsProviderPatchOverrides,
			EnvVar:      "TERRAGRUNT_EXCLUDE_DIR",
			Usage:       "A key=value attribute to override in a provider block as part of the aws-provider-patch command. May be specified multiple times.",
		},
	}
}

func NewCommand(opts *options.TerragruntOptions) *cli.Command {
	return &cli.Command{
		Name:   CommandName,
		Usage:  "Overwrite settings on nested AWS providers to work around a Terraform bug (issue #13018).",
		Flags:  NewFlags(opts).Sort(),
		Action: func(ctx *cli.Context) error { return Run(ctx, opts.OptionsFromContext(ctx)) },
	}
}

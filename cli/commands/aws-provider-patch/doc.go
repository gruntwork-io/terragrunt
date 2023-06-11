package awsproviderpatch

// `aws-provider-patch` command finds all Terraform modules nested in the current code (i.e., in the .terraform/modules
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
// And if AwsProviderPatchOverrides in terragruntOptions was set to map[string]string{"region": "us-east-1"}, then this
// method would update the module code to:
//
//	provider "aws" {
//	   region = "us-east-1"
//	}
//
// This is a temporary workaround for a Terraform bug (https://github.com/hashicorp/terraform/issues/13018) where
// any dynamic values in nested provider blocks are not handled correctly when you call 'terraform import', so by
// temporarily hard-coding them, we can allow 'import' to work.

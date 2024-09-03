provider "aws" {
  region              = var.primary_aws_region
  allowed_account_ids = var.allowed_account_ids
}

module "example_module" {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture-aws-provider-patch/example-module?ref=__BRANCH_NAME__"

  allowed_account_ids  = var.allowed_account_ids
  secondary_aws_region = var.secondary_aws_region
  bucket_name          = var.bucket_name
}

variable "primary_aws_region" {
  description = "The primary AWS region for this module"
  type        = string
}

variable "secondary_aws_region" {
  description = "The secondary AWS region to deploy the S3 bucket from the module into"
  type        = string
}

variable "allowed_account_ids" {
  description = "The list of IDs of AWS accounts that are allowed to run this module."
  type        = list(string)
}

variable "bucket_name" {
  description = "The name to use for the S3 bucket"
  type        = string
}

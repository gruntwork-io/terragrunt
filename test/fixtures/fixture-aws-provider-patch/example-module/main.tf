# We intentionally have an AWS provider block nested within this module so that we can have an integration test that
# checks if the aws-provider-patch command helps to work around https://github.com/hashicorp/terraform/issues/13018.
provider "aws" {
  region              = var.secondary_aws_region
  allowed_account_ids = var.allowed_account_ids
  alias               = "secondary"
}

variable "secondary_aws_region" {
  description = "The AWS region to deploy the S3 bucket into"
  type        = string
}

variable "bucket_name" {
  description = "The name to use for the S3 bucket"
  type        = string
}

variable "allowed_account_ids" {
  description = "The list of IDs of AWS accounts that are allowed to run this module."
  type        = list(string)
}

resource "aws_s3_bucket" "example" {
  bucket = var.bucket_name

  provider = aws.secondary

  # Set to true to make testing easier
  force_destroy = true
}

resource "null_resource" "complex_expression" {
  triggers = (
    var.secondary_aws_region == "us-east-1"
    ? { default = "True" }
    : {}
  )
}

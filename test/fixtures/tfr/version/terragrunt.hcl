# Retrieve a module from the public terraform registry to use with terragrunt
locals {
  base_source = "tfr:///terraform-aws-modules/iam/aws"
  version     = "~>5.0, <5.51.0, !=5.50.0"
}
terraform {
  source = "${local.base_source}?version=${local.version}"
}
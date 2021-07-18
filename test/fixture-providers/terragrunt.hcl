locals {
  aws_region      = "us-east-1"
  assume_role_arn = "arn:aws:iam::1234567890:role/assume-role"
}

generate "test-provider" {
  path      = "test-provider.tf"
  if_exists = "overwrite_terragrunt"

  contents = templatefile("${path_relative_to_include()}/providers.tmpl", {
    aws_region      = local.aws_region,
    assume_role_arn = local.assume_role_arn
  })
}

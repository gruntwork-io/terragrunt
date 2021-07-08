generate "test-provider" {
  path = "test-provider.tf"
  if_exists = "overwrite_terragrunt"

  contents = make_aws_provider({
    region = "us-east-1"
    profile = "test-profile"
  })
}

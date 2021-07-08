generate "test-provider" {
  path      = "test-provider.tf"
  if_exists = "overwrite_terragrunt"

  contents = make_aws_provider({
    region              = "us-east-1"
    version             = "2.3.1"
    allowed_account_ids = ["1234567890"]
  })
}

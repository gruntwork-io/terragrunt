locals {
  aws_region = "us-east-1"
}

generate "test-provider" {
  path      = "test-provider.tf"
  if_exists = "overwrite_terragrunt"

  contents = make_aws_provider({
    region                      = local.aws_region
    allowed_account_ids         = ["1234567890"]
    skip_credentials_validation = true
    assume_role = {
      external_id      = "1"
      duration_seconds = 3600
    }
  })
}

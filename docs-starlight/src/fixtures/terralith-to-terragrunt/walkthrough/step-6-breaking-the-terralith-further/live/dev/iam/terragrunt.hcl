include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//iam"
}

dependency "s3" {
  config_path = "../s3"

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    arn = "arn:aws:s3:::mock-bucket-name"
  }
}

dependency "ddb" {
  config_path = "../ddb"

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    arn = "arn:aws:dynamodb:us-east-1:123456789012:table/mock-table-name"
  }
}

inputs = {
  name = "best-cat-2025-09-24-2359-dev"

  aws_region = "us-east-1"

  s3_bucket_arn      = dependency.s3.outputs.arn
  dynamodb_table_arn = dependency.ddb.outputs.arn
}

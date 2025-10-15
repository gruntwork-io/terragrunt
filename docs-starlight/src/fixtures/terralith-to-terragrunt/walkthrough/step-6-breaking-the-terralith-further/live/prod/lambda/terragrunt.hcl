include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//lambda"
}

dependency "s3" {
  config_path = "../s3"

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    name = "mock-bucket-name"
  }
}

dependency "ddb" {
  config_path = "../ddb"

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    name = "mock-table-name"
  }
}

dependency "iam" {
  config_path = "../iam"

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    arn = "arn:aws:iam::123456789012:role/mock-role-name"
  }
}

inputs = {
  name = "best-cat-2025-09-24-2359"

  aws_region = "us-east-1"

  s3_bucket_name      = dependency.s3.outputs.name
  dynamodb_table_name = dependency.ddb.outputs.name
  lambda_role_arn     = dependency.iam.outputs.arn

  lambda_zip_file     = "${get_repo_root()}/dist/best-cat.zip"
}

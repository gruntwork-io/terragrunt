include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "${find_in_parent_folders("catalog/modules")}//lambda"
}

dependency "s3" {
  config_path = values.s3_path

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    name = "mock-bucket-name"
  }
}

dependency "ddb" {
  config_path = values.ddb_path

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    name = "mock-table-name"
  }
}

dependency "iam" {
  config_path = values.iam_path

  mock_outputs_allowed_terraform_commands = ["plan", "state"]
  mock_outputs_merge_strategy_with_state  = "shallow"

  mock_outputs = {
    arn = "arn:aws:iam::123456789012:role/mock-role-name"
  }
}

inputs = {
  name = values.name

  aws_region = values.aws_region

  s3_bucket_name      = dependency.s3.outputs.name
  dynamodb_table_name = dependency.ddb.outputs.name
  lambda_role_arn     = dependency.iam.outputs.arn

  lambda_zip_file     = "${get_repo_root()}/dist/best-cat.zip"
}

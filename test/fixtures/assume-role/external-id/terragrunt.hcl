remote_state {
  backend  = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket         = "__FILL_IN_BUCKET_NAME__"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-west-2"
    encrypt        = true
    assume_role    = {
      role_arn     = get_aws_caller_identity_arn()
      external_id  = "external_id_123"
      session_name = "session_name_example"
    }
  }
}

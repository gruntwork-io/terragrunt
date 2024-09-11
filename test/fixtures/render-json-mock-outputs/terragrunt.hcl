remote_state {
  backend = "s3"
  config = {
    encrypt                   = true
    bucket                    = "test-tf-state"
    key                       = "${path_relative_to_include()}/terraform.tfstate"
    dynamodb_table            = "test-terraform-locks"
    accesslogging_bucket_name = "test-tf-logs"
  }
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
}

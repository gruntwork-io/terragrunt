locals {
  aws_account_id = "${get_aws_account_id()}"
}

remote_state {
  backend = "s3"
  generate = {
    path      = "backend-${get_aws_account_id()}.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket  = "__FILL_IN_BUCKET_NAME__"
    key     = "${path_relative_to_include()}/terraform.tfstate"
    region  = "__FILL_IN_REGION__"
    encrypt = true
  }
}

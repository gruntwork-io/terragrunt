remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket  = "__FILL_IN_BUCKET_NAME__"
    key     = "${path_relative_to_include()}/terraform.tfstate"
    region  = "__FILL_IN_REGION__"
    encrypt = true

    assume_role_with_web_identity = {
      role_arn                = "__FILL_IN_ASSUME_ROLE__"
      web_identity_token_file = "__FILL_IN_IDENTITY_TOKEN_FILE_PATH__"
    }
  }
}

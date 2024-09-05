iam_role = "__FILL_IN_ASSUME_ROLE__"
iam_web_identity_token = get_env("__FILL_IN_IDENTITY_TOKEN_ENV_VAR__", "")

remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket         = "__FILL_IN_BUCKET_NAME__"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "__FILL_IN_REGION__"
    encrypt        = true
  }
}

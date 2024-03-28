remote_state {
  backend  = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    bucket         = "__FILL_IN_BUCKET_NAME__"
    key            = "${path_relative_to_include()}/terraform.tfstate"
    region         = "us-west-2"
    encrypt        = true
    assume_role    = {
      role_arn     = "__FILL_IN_ASSUME_ROLE__"
      duration     = "1h"
    }
  }
}

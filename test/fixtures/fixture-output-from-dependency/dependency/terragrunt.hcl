# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "s3"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    encrypt = true
    bucket  = "__FILL_IN_BUCKET_NAME__"
    key     = "${path_relative_to_include()}/terraform.tfstate"
    region  = "__FILL_IN_REGION__"
  }
}

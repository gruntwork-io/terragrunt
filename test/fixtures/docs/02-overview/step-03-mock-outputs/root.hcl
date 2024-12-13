remote_state {
  backend = "s3"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    bucket = "__FILL_IN_BUCKET_NAME__"

    key            = "${path_relative_to_include()}/tofu.tfstate"
    region         = "__FILL_IN_REGION__"
    encrypt        = true
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
  }
}

generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "aws" {
  region = "__FILL_IN_REGION__"
}
EOF
}

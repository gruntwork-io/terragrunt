feature "disable_versioning" {
  default = false
}

remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    key            = "${path_relative_to_include()}/tofu.tfstate"
    bucket         = "__FILL_IN_BUCKET_NAME__"
    region         = "__FILL_IN_REGION__"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"

    skip_bucket_versioning = feature.disable_versioning.value
  }
}

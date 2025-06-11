feature "disable_versioning" {
  default = false
}

feature "enable_lock_table_ssencryption" {
  default = false
}

feature "access_logging_bucket" {
  default = ""
}

feature "key_prefix" {
  default = ""
}

remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    key            = "${feature.key_prefix.value}${path_relative_to_include()}/tofu.tfstate"
    bucket         = "__FILL_IN_BUCKET_NAME__"
    region         = "__FILL_IN_REGION__"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"

    skip_bucket_versioning         = feature.disable_versioning.value
    enable_lock_table_ssencryption = feature.enable_lock_table_ssencryption.value
    accesslogging_bucket_name      = feature.access_logging_bucket.value
  }
}

feature "disable_versioning" {
  default = false
}

remote_state {
  backend = "gcs"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    prefix   = "${path_relative_to_include()}/tofu.tfstate"
    location = "__FILL_IN_LOCATION__"
    project  = "__FILL_IN_PROJECT__"
    bucket   = "__FILL_IN_BUCKET_NAME__"

    skip_bucket_versioning = feature.disable_versioning.value
  }
}

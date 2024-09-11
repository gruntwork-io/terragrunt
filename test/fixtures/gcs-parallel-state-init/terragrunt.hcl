remote_state {
  backend = "gcs"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    project        = "__FILL_IN_PROJECT__"
    location       = "__FILL_IN_LOCATION__"
    bucket         = "__FILL_IN_BUCKET_NAME__"
    prefix         = "${path_relative_to_include()}/terraform.tfstate"
  }
}

# Configure Terragrunt to automatically store tfstate files in a GCS bucket
remote_state {
  backend = "gcs"
  config = {
    project  = "__FILL_IN_PROJECT__"
    location = "__FILL_IN_LOCATION__"
    bucket   = "__FILL_IN_BUCKET_NAME__"
    prefix   = "${path_relative_to_include()}"
  }
}

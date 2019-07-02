# Configure Terragrunt to automatically store tfstate files in an existing GCS bucket
remote_state {
  backend = "gcs"

  config = {
    # we are explictly testing that a GCS bucket already exists and terragrunt should
    # work without project and location.
    #project  = ""
    #location = ""
    bucket = "__FILL_IN_BUCKET_NAME__"
    prefix = "terraform.tfstate"
  }
}

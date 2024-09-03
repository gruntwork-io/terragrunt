# Test validation for missing bucket name in configuration
remote_state {
  backend = "gcs"

  config = {
    project  = "__FILL_IN_PROJECT__"
    location = "__FILL_IN_LOCATION__"
    prefix   = "terraform.tfstate"
  }
}

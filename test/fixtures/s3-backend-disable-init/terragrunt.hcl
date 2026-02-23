remote_state {
  backend      = "s3"
  disable_init = true
  config = {
    bucket  = "__FILL_IN_BUCKET_NAME__"
    key     = "terraform.tfstate"
    region  = "__FILL_IN_REGION__"
    encrypt = true
  }
}

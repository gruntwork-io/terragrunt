remote_state {
  backend = "s3"
  config = {
    region = "__FILL_IN_REGION__"
    bucket = "__FILL_IN_BUCKET_NAME__"
    key = "terraform.tfstate"
    encrypt = true
  }
}

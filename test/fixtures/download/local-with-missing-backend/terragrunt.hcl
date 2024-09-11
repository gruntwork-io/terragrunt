inputs = {
  name = "World"
}

terraform {
  source = "../hello-world"
}

# We configure remote state here, but the module in the source parameter does not specify a backend, so we should
# get an error when trying to use this module
remote_state {
  backend = "s3"
  config = {
    encrypt = true
    bucket = "__FILL_IN_BUCKET_NAME__"
    key = "terraform.tfstate"
    region = "us-west-2"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
  }
}
# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "s3"
  config = {
    encrypt = true
    bucket = "my-bucket"
    key = "${path_relative_to_include()}/terraform.tfstate"
    region = "us-east-1"
  }
}


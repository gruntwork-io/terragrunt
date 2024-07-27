# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "s3"
  config = {
    encrypt = true
    bucket  = "__FILL_IN_BUCKET_NAME__"
    key     = "terraform.tfstate"
    region  = "us-west-2"
  }
}

terraform {
  source = "github.com/gruntwork-io/terragrunt.git//test/fixture?ref=v0.21.0"
}

name = "World"

terragrunt = {
  terraform {
    source = "github.com/gruntwork-io/terragrunt.git//test/fixture-download/hello-world-with-backend?ref=v0.12.3"
  }

  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = true
      bucket = "__FILL_IN_BUCKET_NAME__"
      key = "terraform.tfstate"
      region = "us-west-2"
      dynamodb_table = "__FILL_IN_DYNAMODB_TABLE_NAME__"
    }
  }
}
# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "s3"
  config = {
    encrypt = true
    bucket = "__FILL_IN_BUCKET_NAME__"
    key = "terraform.tfstate"
    region = "us-west-2"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
    enable_lock_table_ssencryption = true

    s3_bucket_tags = {
      owner = "terragrunt integration test"
      name = "Terraform state storage"
    }

    dynamodb_table_tags = {
      owner = "terragrunt integration test"
      name = "Terraform lock table"
    }
  }
}

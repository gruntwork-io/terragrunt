terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "terragrunt-test-fixture"
      aws_region = "us-east-1"
      table_name = "terragrunt_locks_test_fixture"
      max_lock_retries = 360
    }
  }

  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "__FILL_IN_BUCKET_NAME__"
      key = "terraform.tfstate"
      region = "us-west-2"
    }
  }
}

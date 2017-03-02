terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "${path_relative_to_include()}"
      table_name = "terragrunt_locks_test_fixture_output_all"
    }
  }

  # Configure Terragrunt to automatically store tfstate files in an S3 bucket
  remote_state {
    backend = "s3"
    config {
      encrypt = "true"
      bucket = "__FILL_IN_BUCKET_NAME__"
      key = "${path_relative_to_include()}/terraform.tfstate"
      region = "us-west-2"
    }
  }
}

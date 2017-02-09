terragrunt = {
  # Configure Terragrunt to use DynamoDB for locking
  lock {
    backend = "dynamodb"
    config {
      state_file_id = "module-c"
    }
  }

  dependencies {
    paths = ["../module-a"]
  }
}

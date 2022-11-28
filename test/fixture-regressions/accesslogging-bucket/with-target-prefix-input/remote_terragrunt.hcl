# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    encrypt = true
    bucket = "__FILL_IN_BUCKET_NAME__"
    key = "terraform.tfstate"
    region = "us-west-2"
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"
    enable_lock_table_ssencryption = true
    accesslogging_bucket_name = "__FILL_IN_LOGS_BUCKET_NAME__"
    accesslogging_target_prefix = "logs/"
    bucket_sse_algorithm = "AES256"
  }
}

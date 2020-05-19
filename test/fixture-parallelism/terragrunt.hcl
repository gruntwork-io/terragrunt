# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "local"
  config = {}
}

inputs = {
  terraform_remote_state_s3_bucket = "__FILL_IN_BUCKET_NAME__"
}

# Configure Terragrunt to automatically store tfstate files in an S3 bucket, but use the `generate` feature to configure
# the remote state
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    encrypt                        = true
    bucket                         = "__FILL_IN_BUCKET_NAME__"
    key                            = "terraform.tfstate"
    region                         = "us-west-2"
    dynamodb_table                 = "__FILL_IN_LOCK_TABLE_NAME__"
    enable_lock_table_ssencryption = true

    s3_bucket_tags = {
      owner = "terragrunt integration test"
      name  = "Terraform state storage"
    }

    dynamodb_table_tags = {
      owner = "terragrunt integration test"
      name  = "Terraform lock table"
    }

    accesslogging_bucket_tags = {
      owner = "terragrunt integration test"
      name  = "Terraform access log storage"
    }
  }
}

terraform {
  source = "../../module"
}

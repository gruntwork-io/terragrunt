# Configure OpenTofu/Terraform state to be stored in S3 with DUAL locking (migration scenario)
# This uses both DynamoDB and S3 native locking simultaneously
# Both locks must be successfully acquired before operations can proceed
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket         = "__FILL_IN_BUCKET_NAME__"
    key            = "dual-locking/terraform.tfstate"
    region         = "__FILL_IN_REGION__"
    encrypt        = true
    dynamodb_table = "__FILL_IN_LOCK_TABLE_NAME__"  # Traditional DynamoDB locking
    use_lockfile   = true                            # New S3 native locking
  }
}

terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/one?version=0.0.2"
}

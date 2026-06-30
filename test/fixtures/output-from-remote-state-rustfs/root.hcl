# Configure Terragrunt to automatically store tfstate files in an S3-compatible bucket (RustFS)
remote_state {
  backend = "s3"
  config = {
    bucket                      = "__FILL_IN_BUCKET_NAME__"
    key                         = "${path_relative_to_include()}/terraform.tfstate"
    region                      = "us-east-1"
    endpoint                    = "__FILL_IN_S3_ENDPOINT__"
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    force_path_style            = true
    encrypt                     = false

    # Skip AWS-specific bucket operations not supported by S3-compatible stores
    skip_bucket_versioning             = true
    skip_bucket_ssencryption           = true
    skip_bucket_accesslogging          = true
    skip_bucket_root_access            = true
    skip_bucket_enforced_tls           = true
    skip_bucket_public_access_blocking = true
  }
}

inputs = {
  terraform_remote_state_s3_bucket = "__FILL_IN_BUCKET_NAME__"
}

# Store state in an S3-compatible bucket (RustFS) with native S3 locking,
# which writes the lock object with a conditional write (If-None-Match).
remote_state {
  backend = "s3"
  config = {
    bucket                      = "__FILL_IN_BUCKET_NAME__"
    key                         = "use-lockfile/terraform.tfstate"
    region                      = "us-east-1"
    endpoint                    = "__FILL_IN_S3_ENDPOINT__"
    skip_credentials_validation = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true
    force_path_style            = true
    encrypt                     = false
    use_lockfile                = true

    # Skip AWS-specific bucket operations not supported by S3-compatible stores
    skip_bucket_versioning             = true
    skip_bucket_ssencryption           = true
    skip_bucket_accesslogging          = true
    skip_bucket_root_access            = true
    skip_bucket_enforced_tls           = true
    skip_bucket_public_access_blocking = true
  }
}

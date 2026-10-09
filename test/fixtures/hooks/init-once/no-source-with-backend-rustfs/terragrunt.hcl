remote_state {
  backend = "s3"
  config = {
    bucket                      = "__FILL_IN_BUCKET_NAME__"
    key                         = "terraform.tfstate"
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

terraform {
  # Should NOT execute. With no source, init-from-module should never execute.
  # If AFTER_INIT_FROM_MODULE_ONLY_ONCE is present in output, the test failed
  after_hook "after_init_from_module" {
    commands = ["init-from-module"]
    execute = ["echo","AFTER_INIT_FROM_MODULE_ONLY_ONCE"]
  }

  # SHOULD execute.
  # If AFTER_INIT_ONLY_ONCE is not echoed exactly once, the test failed
  after_hook "after_init" {
    commands = ["init"]
    execute = ["echo","AFTER_INIT_ONLY_ONCE"]
  }
}

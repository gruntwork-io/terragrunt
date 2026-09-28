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

terraform {
  # This hook configures Terragrunt to attempt to create an empty file called before-parent.out
  # This will be overridden by the child before_hook
  # before execution of terragrunt
  before_hook "before_hook_merge_1" {
    commands = ["apply", "plan"]
    execute = ["touch","before-parent.out"]
    run_on_error = true
  }

  # This hook configures Terragrunt to create an empty file called after-parent.out
  # after execution of terragrunt
  after_hook "after_hook_parent_1" {
    commands = ["apply", "plan"]
    execute = ["touch","after-parent.out"]
    run_on_error = true
  }

  after_hook "produce_error_to_test_error_hook" {
    commands = ["apply"]
    execute = ["exit", "1"]
    run_on_error = true
  }

  # This will be overridden by the child error_hook
  error_hook "error_hook_merge_1" {
    commands = ["apply", "plan"]
    execute = ["touch","error-hook-merge-parent.out"]
    on_errors = [".*"]
  }

  error_hook "error_hook_parent" {
    commands = ["apply", "plan"]
    execute = ["touch","error-hook-parent.out"]
    on_errors = [".*"]
  }
}

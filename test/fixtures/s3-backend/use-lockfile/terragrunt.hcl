include "root" {
  path   = find_in_parent_folders()
  expose = true
}

# Configure OpenTofu/Terraform state to be stored in S3 with native S3 locking
# This uses S3 object conditional writes for state locking, which requires OpenTofu >= 1.10
remote_state {
  backend = "s3"
  config = {
    bucket       = "terragrunt-test-${random_id()}"
    key          = "${path_relative_to_include()}/terraform.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}

terraform {
  source = "${include.root.locals.source_url}"
}

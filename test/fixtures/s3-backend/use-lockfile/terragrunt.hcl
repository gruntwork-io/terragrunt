# Configure OpenTofu/Terraform state to be stored in S3 with native S3 locking
# This uses S3 object conditional writes for state locking, which requires OpenTofu >= 1.10
remote_state {
  backend = "s3"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    bucket       = "__FILL_IN_BUCKET_NAME__"
    key          = "use-lockfile/terraform.tfstate"
    region       = "__FILL_IN_REGION__"
    encrypt      = true
    use_lockfile = true
  }
}

terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/one?version=0.0.2"
}

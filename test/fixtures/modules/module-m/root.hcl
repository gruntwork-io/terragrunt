# Configure Terragrunt to automatically store tfstate files in an S3 bucket
remote_state {
  backend = "s3"
  config = {
    bucket = "bucket"
    key = "${path_relative_to_include()}/terraform.tfstate"
  }
}
terraform {
  source = "..."
}
locals {
  env_vars = read_terragrunt_config("${get_parent_terragrunt_dir()}/env.hcl")
  tier_vars = read_terragrunt_config("tier.hcl")
}

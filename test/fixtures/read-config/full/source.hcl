locals {
  the_answer = 42
}

generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "aws" {
  region = "us-east-1"
}
EOF
}

remote_state {
  backend = "local"
  generate = {
    path = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }
  config = {
    path = "foo.tfstate"
  }
}

terraform {
  source                   = "./delorean"
  include_in_copy          = ["time_machine.*"]
  exclude_from_copy        = ["excluded_time_machine.*"]
  copy_terraform_lock_file = true

  extra_arguments "var-files" {
    commands = ["apply", "plan"]
    required_var_files = ["extra.tfvars"]
    optional_var_files = ["optional.tfvars"]
    env_vars = {
      TF_VAR_custom_var = "I'm set in extra_arguments env_vars"
    }
  }

  before_hook "before_hook_1" {
    commands = ["apply", "plan"]
    execute = ["touch", "before.out"]
    run_on_error = true
  }

  after_hook "after_hook_1" {
    commands = ["apply", "plan"]
    execute = ["touch", "after.out"]
    run_on_error = true
  }
}

dependencies {
  paths = ["../../terragrunt"]
}

terraform_binary = "terragrunt"
terraform_version_constraint = "= 0.12.20"
terragrunt_version_constraint = "= 0.23.18"
download_dir = ".terragrunt-cache"
prevent_destroy = true
skip = true
iam_role = "TerragruntIAMRole"

inputs = {
  doc = "Emmett Brown"
}

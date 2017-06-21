terragrunt = {
  # This include stanza utilizes the find_in_parent_folders() Terragrunt built-in,
  # which instructs Terragrunt to locate the root terraform.tfvars
  # file and copy that configuration. This allows us to remain DRY and prevents
  # the need to copy/paste general configuration, such as remote state settings.
  include {
    path = "${find_in_parent_folders()}"
  }

  # Download and apply the Terraform source code from the following GitHub repo.
  # Notice that a specific version of the repo is defined. This paradigm makes
  # it straight forward to develop and test changes to Terraform modules in
  # staging environments before updating the production environment.
  terraform {
    source = "git::git@github.com:conorgil/terragrunt_sample_frontend_app.git?ref=v0.0.1"
  }
}

# Define values for variables which are specific to the terragrunt_sample_frontend_app
# terraform module in the PRODUCTION environment.

ami_id = "ami-a025aeb6"

instance_count = "9"

instance_type = "t2.large"

remote_state_bucket = "terragrunt-examples-remote-state"

# TODO replace hardcoded production with a variable injected with extra_variables
vpc_remote_state_key = "production/vpc/terraform.tfstate"

remote_state_lock_table = "terragrunt-examples-lock-table"

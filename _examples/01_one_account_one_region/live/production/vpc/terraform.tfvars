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
    source = "git::git@github.com:conorgil/terragrunt_sample_vpc.git?ref=v0.0.4"
  }
}

# Define values for variables which are specific to the terragrunt_sample_vpc
# terraform module in the PRODUCTION environment.

cidr_block = "10.0.0.0/16"

subnet_bit_size = "8"

public_subnet_count = "3"

private_subnet_count = "3"

# This terraform.tfvars file sits at the root of the live/ directory
# and defines general configuration that applies to all components in
# our infrastructure. We will call this file the "root configuration".
# Each subdirectory defines a single environment of the overall infrastructure
# (e.g. production, staging). For example, the following directory tree contains
# a single root configuration (live/terraform.tfvars) and 2 environments 
# (production and staging):
#
# live
# ├── production
# ├── staging
# └── terraform.tfvars

# Each environment directory does not normally contain any files. The environment
# directory is the location where Terragrunt commands such as plan-all and apply-all
# can be executed to instantiate all of the infrastructure for that environment.
# The environment directory contains a child directory for each component of 
# the infrastructure. We will call these child directories "component directories".
# For example, the following directory tree contains 6 component directories;
# 3 for production (vpc, fronent_app, mysql) and the same 3 for staging.
#
# live
# ├── production
# │   ├── frontend-app
# │   ├── mysql
# │   └── vpc
# ├── staging
# │   ├── frontend-app
# │   ├── mysql
# │   └── vpc
# └── terraform.tfvars

# Each component directory contains a single terraform.tfvars file, which
# defines the configuration for that specific component. We will call this
# terraform.tfvars file the "component configuration". Each component configuration
# can contain an include stanza to instruct Terragrunt to locate the root configuration
# and copy all Terragrunt configuration. This allows us to follow the same patterns
# in all of our components and keep our configuration DRY.
# For example, the following directory tree contains 6 component configurations;
# 3 for the production components and 3 for the staging components:
#
# live
# ├── production
# │   ├── frontend-app
# │   |   └── terraform.tfvars
# │   ├── mysql
# │   |   └── terraform.tfvars
# │   └── vpc
# │       └── terraform.tfvars
# ├── staging
# │   ├── frontend-app
# │   |   └── terraform.tfvars
# │   ├── mysql
# │   |   └── terraform.tfvars
# │   └── vpc
# │       └── terraform.tfvars
# └── terraform.tfvars

# The following root configuration defines the remote_state configuration
# a single time, which is then used by all component configurations.

terragrunt = {
  remote_state {
    backend = "s3"

    config {
      # We can store all of our terraform remote states in a single S3 bucket
      bucket = "terragrunt-examples-remote-state"

      # The path_relative_to_include() is a Terragrunt built-in function,
      # which will provide the relative path from the root terraform.tfvars
      # file to each environment terraform.tfvars file. This allows us to 
      # easily store the Terraform state for each environment in its own
      # subfolder of the S3 bucket. For example, 
      # terragrunt-examples-remote-state/production/vpc/terraform.tfstate
      # and terragrunt-examples-remote-state/staging/vpc/terraform.tfstate.
      key = "${path_relative_to_include()}/terraform.tfstate"

      # The AWS region in which to create the S3 bucket for the Terraform
      # remote state.
      region = "us-east-1"

      # It is best practice to use S3 encryption so that your Terraform state
      # files are encrypted at rest in the S3 bucket. Remember that currently
      # Terraform state files can contain cleartext credentials depending on
      # which Terraform providers you have configured.
      encrypt = true

      # The lock_table tells Terraform which DynamoDB table to use
      # as the shared locking mechanism. Remote state locking is natively
      # supported by Terraform.
      lock_table = "terragrunt-examples-lock-table"
    }
  }
}

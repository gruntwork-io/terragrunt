# This root configuration defines the settings for the S3 backend.
# Component configurations throughout the infrastructure can use an
# include stanza to copy these settings, thus keeping configurations DRY.

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

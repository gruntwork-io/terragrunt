# A dirt simple module for use with the custom-lock-file test

# This provider is not actually used in the module, but by having it here, Terraform will download the code for it
# when we run 'init'. If the lock file copying works as expected in the custom-lock-file test, then we'll end up
# with an older version of the provider. If there is a bug, Terraform will end up downloading the latest version of
# the provider, as we're not pinning the version in the Terraform code (only in the lock file).
provider "aws" {
  region = "eu-west-1"
}

variable "name" {
  description = "The name to use"
  type        = string
}

output "text" {
  value = "Hello, ${var.name}"
}
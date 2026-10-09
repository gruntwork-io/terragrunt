# A dirt simple module for use with the custom-lock-file test

# This provider is not actually used in the module, but by having it here, Terraform will download the code for it
# when we run 'init'.
terraform {
  required_providers {
    aws = {
      source  = "registry.opentofu.org/hashicorp/aws"
      version = "6.56.0"
    }
  }
}

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
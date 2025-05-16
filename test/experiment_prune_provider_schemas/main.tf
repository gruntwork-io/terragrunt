terraform {
  required_providers {
    aws         = { source = "hashicorp/aws" }
    awscc       = { source = "hashicorp/awscc" }
    azurerm     = { source = "hashicorp/azurerm" }
    google      = { source = "hashicorp/google" }
    google-beta = { source = "hashicorp/google-beta" }
  }
}

output "foo" {
  value = "bar"
}
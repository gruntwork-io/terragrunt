terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "5.36.0"
    }
    azure = {
      source  = "hashicorp/azurerm"
      version = "3.95.0"
    }
  }
}

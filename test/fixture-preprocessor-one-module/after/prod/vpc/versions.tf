terraform {
  required_version = "1.2.3"

  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "~> 3.0"
    }
  }

  backend "local" {
    path = "prod/vpc/terraform.tfstate"
  }
}
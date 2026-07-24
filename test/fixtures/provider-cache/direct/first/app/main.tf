terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "3.2.3"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.5.2"
    }
  }
}

module "naming" {
  source  = "cloudposse/label/null"
  version = "0.25.0"
}

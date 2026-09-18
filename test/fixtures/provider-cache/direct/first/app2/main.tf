terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "3.2.4"
    }
    local = {
      source  = "hashicorp/local"
      version = "2.6.1"
    }
  }
}


module "naming" {
  source = "cloudposse/label/null"
}

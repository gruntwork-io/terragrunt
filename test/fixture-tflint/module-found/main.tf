terraform {
  required_version = ">= 1.2.7"
  required_providers {
  }
}

module "dummy_module" {
  source = "./dummy_module"
}

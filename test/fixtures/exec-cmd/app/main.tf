terraform {
  required_providers {
    null = {
      source  = "registry.terraform.io/hashicorp/null"
      version = "2.1.2"
    }
  }
}

variable foo {
  type = string
}

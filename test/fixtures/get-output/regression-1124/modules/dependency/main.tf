terraform {
  required_providers {
    random = {
      source  = "registry.opentofu.org/hashicorp/random"
      version = "3.8.0"
    }
  }
}

resource "random_string" "random" {
  length = 16
}

output "foo" {
  value = random_string.random.result
}

terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "dns" {
  triggers = {
    name = "dns"
  }
}

# Deliberately unlike the mock the dependent declares, so a test can tell an
# output read out of this unit's state from a mock standing in for it.
output "dns_id" {
  value = "dns-applied"
}

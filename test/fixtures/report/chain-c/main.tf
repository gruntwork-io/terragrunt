terraform {
  required_providers {
    null = {
      source  = "registry.opentofu.org/hashicorp/null"
      version = "3.2.4"
    }
  }
}

resource "null_resource" "chain_c" {
  triggers = {
    depends_on_b = dependency.chain_b.outputs
  }
}

include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "tfr:///Azure/avm-res-resources-resourcegroup/azurerm?version=0.4.0"
}

inputs = {
  name     = "__FILL_IN_NAME_PREFIX__-step-two-rg"
  location = "__FILL_IN_LOCATION__"

  tags = {
    IaC = "true"
    Environment = "dev"
  }
}

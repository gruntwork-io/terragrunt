include "root" {
  path = find_in_parent_folders("root.hcl")
}

terraform {
  source = "tfr:///Azure/avm-res-resources-resourcegroup/azurerm?version=0.4.0"
}

inputs = {
  name     = "__FILL_IN_NAME_PREFIX__-step-four-rg"
  location = "eastus"

  tags = {
    IaC = "true"
    Environment = "dev"
  }
}

# Configure the remote backend
remote_state {
  backend = "azurerm"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite_terragrunt"
  }

  config = {
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP__"
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT__"
    container_name       = "tfstate"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"

    key                   = "tofu.tfstate"
    location              = "__FILL_IN_LOCATION__"
    use_azuread_auth      = true
    assign_blob_data_role = true
  }
}

# Configure the Azure provider
generate "provider" {
  path = "provider.tf"
  if_exists = "overwrite_terragrunt"
  contents = <<EOF
provider "azapi" {
  subscription_id  = "__FILL_IN_SUBSCRIPTION_ID__"
  default_location = "__FILL_IN_LOCATION__"
}
EOF
}

# Configure the module
#
# This URL is shorthand for
# "tfr://registry.terraform.io/Azure/avm-res-resources-resourcegroup/azurerm?version=0.4.0".
#
# The module lives at:
# https://registry.terraform.io/modules/Azure/avm-res-resources-resourcegroup/azurerm/latest
#
# The shorthand needs the extra `/` after `tfr`.
terraform {
  source = "tfr:///Azure/avm-res-resources-resourcegroup/azurerm?version=0.4.0"
}

# Configure the inputs for the module
inputs = {
  name     = "__FILL_IN_NAME_PREFIX__-step-one-rg"
  location = "__FILL_IN_LOCATION__"

  tags = {
    IaC = "true"
    Environment = "dev"
  }
}

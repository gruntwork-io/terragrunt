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

    key                   = "${path_relative_to_include()}/tofu.tfstate"
    location              = "__FILL_IN_LOCATION__"
    use_azuread_auth      = true
    assign_blob_data_role = true
  }
}

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

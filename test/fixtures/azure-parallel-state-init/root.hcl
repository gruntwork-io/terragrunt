remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP_NAME__"
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT_NAME__"
    container_name       = "__FILL_IN_CONTAINER_NAME__"
    key                  = "${path_relative_to_include()}/terraform.tfstate"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"
    use_azuread_auth     = true
  }
}

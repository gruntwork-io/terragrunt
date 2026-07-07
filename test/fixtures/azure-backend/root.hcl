remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT__"
    container_name       = "__FILL_IN_CONTAINER__"
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP__"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"
    location             = "__FILL_IN_LOCATION__"
    use_azuread_auth     = true
    key                  = "${path_relative_to_include()}/terraform.tfstate"
  }
}

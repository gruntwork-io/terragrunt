remote_state {
  backend = "azurerm"
  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }
  config = {
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT_NAME__"
    container_name      = "__FILL_IN_CONTAINER_NAME__"
    use_azuread_auth   = true
    subscription_id    = "__FILL_IN_SUBSCRIPTION_ID__"
    key               = "${path_relative_to_include()}/terraform.tfstate"
  }
}
# Configure Terragrunt to automatically store tfstate files in Azure Storage
remote_state {
  backend = "azurerm"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT_NAME__"
    container_name       = "__FILL_IN_CONTAINER_NAME__"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP_NAME__"
    key                  = "${path_relative_to_include()}/terraform.tfstate"
    use_azuread_auth     = true
  }
}

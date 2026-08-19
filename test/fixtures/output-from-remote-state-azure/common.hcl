remote_state {
  backend = "azurerm"

  config = {
    key                  = "${path_relative_to_include()}/terraform.tfstate"
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT__"
    container_name       = "__FILL_IN_CONTAINER__"
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP__"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"

    location                 = "__FILL_IN_LOCATION__"
    account_tier             = "Standard"
    account_replication_type = "LRS"
    account_kind             = "StorageV2"
    access_tier              = "Hot"
  }
}

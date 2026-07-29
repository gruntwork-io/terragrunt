feature "disable_versioning" {
  default = false
}

feature "key_prefix" {
  default = ""
}

remote_state {
  backend = "azurerm"

  config = {
    key                  = "${feature.key_prefix.value}${path_relative_to_include()}/tofu.tfstate"
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT__"
    container_name       = "__FILL_IN_CONTAINER__"
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP__"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"
    use_azuread_auth     = true

    # Terragrunt-only bootstrap keys. These must be consumed by Terragrunt and
    # filtered out before `tofu init -backend-config`, which rejects unknown
    # arguments. Keeping them here means the E2E run proves that filtering.
    location                 = "__FILL_IN_LOCATION__"
    account_tier             = "Standard"
    account_replication_type = "LRS"
    account_kind             = "StorageV2"
    access_tier              = "Hot"

    skip_versioning = feature.disable_versioning.value
  }
}

feature "disable_versioning" {
  default = false
}

feature "key_prefix" {
  default = ""
}

remote_state {
  backend = "azurerm"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    key                  = "${feature.key_prefix.value}${path_relative_to_include()}/terraform.tfstate"
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT_NAME__"
    container_name       = "__FILL_IN_CONTAINER_NAME__"
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP_NAME__"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"
    use_azuread_auth     = true

    skip_metadata_api_check = feature.disable_versioning.value
  }
}

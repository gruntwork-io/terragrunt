terraform {
  source = "tfr://registry.terraform.io/yorinasub17/terragrunt-registry-test/null//modules/one?version=0.0.2"
}

remote_state {
  backend = "azurerm"

  generate = {
    path      = "backend.tf"
    if_exists = "overwrite"
  }

  config = {
    key                  = "unit2/terraform.tfstate"
    storage_account_name = "__FILL_IN_STORAGE_ACCOUNT_NAME__"
    container_name       = "__FILL_IN_CONTAINER_NAME__"
    resource_group_name  = "__FILL_IN_RESOURCE_GROUP_NAME__"
    subscription_id      = "__FILL_IN_SUBSCRIPTION_ID__"
    use_azuread_auth     = true
  }
}

terraform {
  backend "azurerm" {}
}

output "producer_value" {
  value = "from-azure-state"
}

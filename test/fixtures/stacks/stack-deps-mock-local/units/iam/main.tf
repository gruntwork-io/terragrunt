variable "account_name" {
  type    = string
  default = ""
}

output "account_name" {
  value = var.account_name
}

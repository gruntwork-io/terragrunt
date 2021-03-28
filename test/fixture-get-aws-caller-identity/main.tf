variable "account" {
  type = string
}

variable "account_alias" {
  type = string
}

variable "arn" {
  type = string
}

variable "user_id" {
  type = string
}

output "account" {
  value = var.account
}

output "account_alias" {
  value = var.account_alias
}

output "arn" {
  value = var.arn
}

output "user_id" {
  value = var.user_id
}

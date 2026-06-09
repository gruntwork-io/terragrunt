variable "account_name" {
  type = string
}

variable "region" {
  type = string
}

output "ok" {
  value = "${var.account_name}-${var.region}"
}

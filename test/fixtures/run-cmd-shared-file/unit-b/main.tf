variable "account" {
  type = string
}

variable "joined" {
  type = string
}

variable "split" {
  type = string
}

output "account" {
  value = var.account
}

output "joined" {
  value = var.joined
}

output "split" {
  value = var.split
}

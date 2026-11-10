variable "network_id" {}

variable "network" {
  type = object({ id = string, cidr = string })
}

output "network_id" {
  value = var.network_id
}

output "network" {
  value = var.network
}

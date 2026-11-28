variable "cidr" {}

output "network" {
  value = { id = "vpc-1", cidr = var.cidr }
}

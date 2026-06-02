variable "availability_zones" {
  type    = list(string)
  default = []
}

output "azs" {
  value = var.availability_zones
}

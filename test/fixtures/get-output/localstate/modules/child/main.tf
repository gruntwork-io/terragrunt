terraform {
  backend "local" {}
}

variable "x" {}

output "y" {
  value = var.x * 3
}

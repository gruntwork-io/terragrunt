variable "x" {}

variable "y" {}

output "z" {
  value = var.x + var.y
}

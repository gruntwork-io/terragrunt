variable "i1" {
  type = bool
}

variable "i2" {
  type = bool
}

output "o1" {
  value = var.i1
}

output "o2" {
  value = var.i2
}

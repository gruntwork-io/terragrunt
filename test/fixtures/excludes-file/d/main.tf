variable "value" {
  type    = string
  default = "d"
}

output "value" {
  value = var.value
}

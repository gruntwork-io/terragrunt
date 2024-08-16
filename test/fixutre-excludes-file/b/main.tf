variable "value" {
  type    = string
  default = "b"
}

output "value" {
  value = var.value
}

variable "value" {
  type    = string
  default = "a"
}

output "value" {
  value = var.value
}

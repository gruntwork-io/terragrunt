variable "value" {
  type    = string
  default = "ok"
}

output "value" {
  value = var.value
}

variable "value" {
  type    = string
  default = "c"
}

output "value" {
  value = var.value
}

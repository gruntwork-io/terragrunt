variable "dep_value" {
  type = string
}

output "result" {
  value = "app got ${var.dep_value}"
}


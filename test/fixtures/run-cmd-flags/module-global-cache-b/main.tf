variable "cached_value" {
  type = string
}

output "cached_value_b" {
  value = var.cached_value
}

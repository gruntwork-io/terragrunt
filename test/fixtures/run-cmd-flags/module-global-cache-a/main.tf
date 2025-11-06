variable "cached_value" {
  type = string
}

output "cached_value_a" {
  value = var.cached_value
}

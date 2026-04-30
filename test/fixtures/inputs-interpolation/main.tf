variable "map_with_interpolation" {
  type = map(string)
}

output "map_with_interpolation" {
  value = var.map_with_interpolation
}

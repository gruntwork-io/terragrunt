variable "map_with_interpolation" {
  type = map(string)
}

variable "string_with_interpolation" {
  type = string
}

variable "any_with_interpolation" {
  type = any
}

variable "string_from_map" {
  type = string
}

output "map_with_interpolation" {
  value = var.map_with_interpolation
}

output "string_with_interpolation" {
  value = var.string_with_interpolation
}

output "any_with_interpolation" {
  value = var.any_with_interpolation
}

output "string_from_map" {
  value = jsondecode(var.string_from_map)
}

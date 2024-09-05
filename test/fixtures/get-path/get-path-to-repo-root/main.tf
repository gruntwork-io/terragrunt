variable "path_to_root" {
  type = string
}

variable "path_to_modules" {
  type = string
}

output "path_to_root" {
  value = var.path_to_root
}

output "path_to_modules" {
  value = var.path_to_modules
}

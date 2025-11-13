variable "password" {
  type      = string
  sensitive = true
}

output "password_length" {
  value     = length(var.password)
  sensitive = true
}

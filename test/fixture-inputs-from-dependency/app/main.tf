variable "foo" {
  type    = string
  default = "app-foo-value"
}

output "foo" {
  value       = var.foo
  description = "Test foo value"
}

variable "bar" {
  type    = string
  default = "app-bar-value"
}

output "bar" {
  value       = var.bar
  description = "Test bar value"
}

variable "baz" {
  type    = string
  default = "app-baz-value"
}

output "baz" {
  value       = var.baz
  description = "Test baz value"
}

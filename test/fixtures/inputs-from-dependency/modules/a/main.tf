variable "foo" {
  type    = string
  default = "empty"
}

output "foo" {
  value       = var.foo
  description = "Test foo value"
}

variable "bar" {
  type    = string
  default = "empty"
}

output "bar" {
  value       = var.bar
  description = "Test bar value"
}

variable "baz" {
  type    = string
  default = "empty"
}

output "baz" {
  value       = var.baz
  description = "Test baz value"
}

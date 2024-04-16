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

variable "dep-output-test" {
  type    = string
  default = "empty"
}

output "dep-output-test" {
  value       = var.dep-output-test
  description = "Test dep-output-test value"
}

variable "dep-cluster-id" {
  type    = string
  default = "empty"
}

output "dep-cluster-id" {
  value       = var.dep-cluster-id
  description = "Test dep-cluster-id value"
}

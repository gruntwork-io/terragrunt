variable "test" {
  type    = string
  default = "test-value"
}

output "test" {
  value       = var.test
  description = "Test value"
}

variable "cluster-id" {
  type    = string
  default = "empty"
}

output "cluster-id" {
  value       = var.cluster-id
  description = "Test cluster-id value"
}

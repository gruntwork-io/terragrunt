variable "test" {
  type        = string
  default     = "test-value"
}

output "test" {
  value       = var.test
  description = "Test value"
}

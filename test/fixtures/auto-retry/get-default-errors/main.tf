variable "retryable_errors" {
  type = list(any)
}

output "retryable_errors" {
  value = var.retryable_errors
}

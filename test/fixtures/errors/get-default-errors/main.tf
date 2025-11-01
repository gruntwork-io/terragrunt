variable "default_retryable_errors" {
  type = list(string)
}

variable "custom_error" {
  type = string
}

output "default_retryable_errors" {
  value = var.default_retryable_errors
}

output "custom_error" {
  value = var.custom_error
}

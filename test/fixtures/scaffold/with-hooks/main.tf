# Test module for hooks testing
variable "test_var" {
  description = "Test variable"
  type        = string
  default     = ""
}

output "test_output" {
  value = var.test_var
}


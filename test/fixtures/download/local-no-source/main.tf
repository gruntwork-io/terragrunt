# Simple terraform file for testing cache creation without source

variable "test_value" {
  description = "Test value"
  type        = string
  default     = "default"
}

output "test_output" {
  value = var.test_value
}

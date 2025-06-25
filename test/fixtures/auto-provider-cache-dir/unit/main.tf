terraform {
  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
  }
}

variable "test_input" {
  description = "Test input variable"
  type        = string
  default     = "test"
}

resource "null_resource" "test" {
  triggers = {
    test_input = var.test_input
  }
}

output "test_output" {
  value = null_resource.test.triggers.test_input
}

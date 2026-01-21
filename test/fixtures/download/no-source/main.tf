terraform {
  required_version = ">= 1.5.7"

  required_providers {
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2.4"
    }
  }
}

variable "name" {
  description = "Specify a name"
  type        = string
  default     = "no-source-test"
}

resource "null_resource" "test" {
}

output "test" {
  value = "Hello, ${var.name}"
}

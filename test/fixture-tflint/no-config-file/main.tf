terraform {
  required_providers {
    random = {
      version = ">= 3.4.0"
    }
  }

  required_version = ">= 1.2.7"
}

// It's all in the same file, so tflint will return issues.
variable "aws_region" {
  type        = string
  description = "The AWS region."
}

variable "env" {
  type        = string
  description = "The environment name."
}

resource "random_id" "env" {
  byte_length = 8
}

output "aws_region" {
  description = "The AWS region's name."
  value       = var.aws_region
}
output "env" {
  description = "The randomized environment's name."
  value       = "${var.env}-${random_id.env.hex}"
}

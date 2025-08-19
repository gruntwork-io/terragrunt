variable "aws_region" {
  description = "AWS region for all resources"
  type        = string
  default     = "us-east-1"
}

variable "lambda_runtime" {
  description = "Lambda function runtime"
  type        = string
  default     = "nodejs22.x"
}

variable "lambda_handler" {
  description = "Lambda function handler"
  type        = string
  default     = "index.handler"
}

variable "lambda_timeout" {
  description = "Lambda function timeout in seconds"
  type        = number
  default     = 30
}

variable "lambda_memory_size" {
  description = "Lambda function memory size in MB"
  type        = number
  default     = 128
}

variable "lambda_architectures" {
  description = "Lambda function architectures"
  type        = list(string)
  default     = ["arm64"]
}

variable "force_destroy" {
  description = "Force destroy S3 buckets (only set to true for testing or cleanup of demo environments)"
  type        = bool
  default     = false
}

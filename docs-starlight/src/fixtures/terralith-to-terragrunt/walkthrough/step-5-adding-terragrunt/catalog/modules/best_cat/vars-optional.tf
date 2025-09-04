variable "aws_region" {
  description = "AWS region for all resources"
  type        = string
  default     = "us-east-1"
}

variable "force_destroy" {
  description = "Force destroy S3 buckets (only set to true for testing or cleanup of demo environments)"
  type        = bool
  default     = false
}

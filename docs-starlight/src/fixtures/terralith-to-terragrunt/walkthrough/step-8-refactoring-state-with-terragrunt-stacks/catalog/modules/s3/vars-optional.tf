variable "force_destroy" {
  description = "Force destroy S3 buckets (only set to true for testing or cleanup of demo environments)"
  type        = bool
  default     = false
}

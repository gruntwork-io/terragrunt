output "get_caller_identity_arn_output" {
  value = var.get_caller_identity_arn
}

variable "get_caller_identity_arn" {
  description = "get caller identity arn"
  default     = ""
}

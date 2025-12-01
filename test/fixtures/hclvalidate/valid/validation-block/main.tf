variable "example_var" {
  type        = string
  description = "A variable that requires validation."
  default     = "test"

  validation {
    condition     = length(var.example_var) > 0
    error_message = "The example_var must not be empty."
  }
}

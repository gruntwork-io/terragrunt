# Test module for shell command testing
variable "shell_output_1" {
  description = "Output from shell command 1"
  type        = string
  default     = ""
}

variable "shell_output_2" {
  description = "Output from shell command 2"
  type        = string
  default     = ""
}

output "shell_result_1" {
  value = var.shell_output_1
}

output "shell_result_2" {
  value = var.shell_output_2
}


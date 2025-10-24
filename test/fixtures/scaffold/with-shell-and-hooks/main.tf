# Test module for shell and hooks testing
variable "shell_result_1" {
  description = "Output from shell command 1"
  type        = string
  default     = ""
}

variable "shell_result_2" {
  description = "Output from shell command 2"
  type        = string
  default     = ""
}

output "shell_output_1" {
  value = var.shell_result_1
}

output "shell_output_2" {
  value = var.shell_result_2
}


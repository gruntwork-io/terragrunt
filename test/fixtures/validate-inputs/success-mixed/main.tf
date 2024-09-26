variable "input_from_env_var" {}
variable "input_from_input" {}
variable "input_from_varfile" {}

output "output" {
  value = <<-EOF
  ${var.input_from_env_var}
  ${var.input_from_input}
  ${var.input_from_varfile}
  EOF
}

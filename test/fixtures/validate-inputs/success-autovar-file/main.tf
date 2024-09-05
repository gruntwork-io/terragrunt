variable "input_terraformtfvars" {}
variable "input_terraformtfvarsjson" {}
variable "input_autotfvars" {}
variable "input_autotfvarsjson" {}

output "output" {
  value = <<-EOF
  ${var.input_terraformtfvars}
  ${var.input_terraformtfvarsjson}
  ${var.input_autotfvars}
  ${var.input_autotfvarsjson}
  EOF
}

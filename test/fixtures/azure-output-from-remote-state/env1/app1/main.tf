variable "app2_output" {}
variable "app3_output" {}

output "combined_output" {
  value = "app1 output with ${var.app2_output} and ${var.app3_output}"
}

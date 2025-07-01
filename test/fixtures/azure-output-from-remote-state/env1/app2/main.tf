variable "app3_output" {}

output "app2_output" {
  value = "app2 output with ${var.app3_output}"
}

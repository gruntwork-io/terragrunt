variable "app1_text" {}

output "app2_text" {
  value = "app2 output: ${var.app1_text}"
}

variable "app1_output" {
  type = string
}

output "passthrough" {
  value = var.app1_output
}

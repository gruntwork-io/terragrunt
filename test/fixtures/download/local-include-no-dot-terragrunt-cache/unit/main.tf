variable "test_value" {
  type    = string
  default = "default"
}

output "test_output" {
  value = var.test_value
}

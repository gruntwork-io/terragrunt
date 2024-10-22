variable "test_var" {
  type = string
}

output "result" {
  value = var.test_var
}

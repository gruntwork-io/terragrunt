variable "test_var" {
  default = "module1"
}

output "module_name" {
  value = var.test_var
}


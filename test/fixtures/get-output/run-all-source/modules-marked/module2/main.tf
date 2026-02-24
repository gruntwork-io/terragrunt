variable "test_var" {
  default = "module2"
}

output "module_name" {
  value = var.test_var
}


variable "extra_var" {
  description = "Should be loaded from extra.tfvars"
}

output "test" {
  value = var.extra_var
}

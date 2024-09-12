variable "custom_var" {
  description = "Provided as TF_VAR_custom_var in terraform.tfvars"
}

output "test" {
  value = var.custom_var
}

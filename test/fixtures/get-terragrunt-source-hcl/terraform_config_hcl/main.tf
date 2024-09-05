variable "terragrunt_source" {
  type = string
}

output "terragrunt_source" {
  value = "HCL: ${var.terragrunt_source}"
}

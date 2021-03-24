variable "terragrunt_source" {
  type = string
}

output "terragrunt_source" {
  value = "CLI: ${var.terragrunt_source}"
}

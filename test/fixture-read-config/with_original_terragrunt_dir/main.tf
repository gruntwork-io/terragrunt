variable "terragrunt_dir" {
  type = string
}

variable "original_terragrunt_dir" {
  type = string
}

output "terragrunt_dir" {
  value = var.terragrunt_dir
}

output "original_terragrunt_dir" {
  value = var.original_terragrunt_dir
}

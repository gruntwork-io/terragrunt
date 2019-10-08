variable "parent_terragrunt_dir" {}
variable "terragrunt_dir" {}

output "parent_terragrunt_dir" {
  value = var.parent_terragrunt_dir
}

output "terragrunt_dir" {
  value = var.terragrunt_dir
}

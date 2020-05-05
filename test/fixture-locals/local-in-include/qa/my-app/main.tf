variable "parent_terragrunt_dir" {}
variable "terragrunt_dir" {}
variable "terraform_command" {}

output "parent_terragrunt_dir" {
  value = var.parent_terragrunt_dir
}

output "terragrunt_dir" {
  value = var.terragrunt_dir
}

output "terraform_command" {
  value = var.terraform_command
}

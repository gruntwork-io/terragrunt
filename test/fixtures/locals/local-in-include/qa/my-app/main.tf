variable "parent_terragrunt_dir" {}
variable "terragrunt_dir" {}
variable "terraform_command" {}
variable "terraform_cli_args" {}

output "parent_terragrunt_dir" {
  value = var.parent_terragrunt_dir
}

output "terragrunt_dir" {
  value = var.terragrunt_dir
}

output "terraform_command" {
  value = var.terraform_command
}

output "terraform_cli_args" {
  value = var.terraform_cli_args
}
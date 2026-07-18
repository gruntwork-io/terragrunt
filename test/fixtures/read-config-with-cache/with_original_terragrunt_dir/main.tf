variable "terragrunt_dir" {
  type = string
}

variable "original_terragrunt_dir" {
  type = string
}

variable "dep_terragrunt_dir" {
  type = string
}

variable "dep_original_terragrunt_dir" {
  type = string
}

variable "dep_bar_terragrunt_dir" {
  type = string
}

variable "dep_bar_original_terragrunt_dir" {
  type = string
}

output "terragrunt_dir" {
  value = var.terragrunt_dir
}

output "original_terragrunt_dir" {
  value = var.original_terragrunt_dir
}

output "dep_terragrunt_dir" {
  value = var.dep_terragrunt_dir
}

output "dep_original_terragrunt_dir" {
  value = var.dep_original_terragrunt_dir
}

output "dep_bar_terragrunt_dir" {
  value = var.dep_bar_terragrunt_dir
}

output "dep_bar_original_terragrunt_dir" {
  value = var.dep_bar_original_terragrunt_dir
}

variable "localstg" {}
output "localstg" { value = var.localstg }

variable "generate" {}
output "generate" { value = var.generate }

variable "remote_state" {}
output "remote_state" { value = var.remote_state }

variable "terraformtg" {}
output "terraformtg" { value = var.terraformtg }

variable "dependencies" {}
output "dependencies" { value = var.dependencies }

variable "terraform_binary" {}
output "terraform_binary" { value = var.terraform_binary }

variable "terraform_version_constraint" {}
output "terraform_version_constraint" { value = var.terraform_version_constraint }

variable "terragrunt_version_constraint" {}
output "terragrunt_version_constraint" { value = var.terragrunt_version_constraint }

variable "download_dir" {}
output "download_dir" { value = var.download_dir }

variable "prevent_destroy" {}
output "prevent_destroy" { value = var.prevent_destroy }

variable "skip" {}
output "skip" { value = var.skip }

variable "iam_role" {}
output "iam_role" { value = var.iam_role }

variable "inputs" {}
output "inputs" { value = var.inputs }

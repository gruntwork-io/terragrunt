variable "aws_region" {}
variable "env" {}
variable "name" {}
variable "type" {}

output "aws_region" {
  value = var.aws_region
}
output "env" {
  value = var.env
}
output "name" {
  value = var.name
}
output "type" {
  value = var.type
}

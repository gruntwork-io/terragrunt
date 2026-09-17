variable "name" {
  type = string
}

output "subnet_id" {
  value = "subnet-from-${var.name}"
}

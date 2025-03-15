variable "ver" {}

output "data" {
  value = "db ${var.ver}"
}
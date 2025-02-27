variable "ver" {}

output "data" {
  value = "api ${var.ver}"
}
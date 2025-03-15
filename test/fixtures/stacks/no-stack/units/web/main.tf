variable "ver" {}

output "data" {
  value = "web ${var.ver}"
}
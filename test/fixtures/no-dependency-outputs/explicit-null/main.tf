variable "x" {}

output "x" {
  value = var.x == null ? "null" : "set"
}

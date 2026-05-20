variable "message" {
  type    = string
  default = "hello from unit"
}

output "message" {
  value = var.message
}

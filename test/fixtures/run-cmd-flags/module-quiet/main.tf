variable "secret" {
  type = string
}

output "quiet_secret" {
  value = var.secret
}

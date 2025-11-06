variable "test_value" {
  type = string
}

output "secrets" {
  value = {
    test_provider_token = var.test_value
  }
  sensitive = true
}

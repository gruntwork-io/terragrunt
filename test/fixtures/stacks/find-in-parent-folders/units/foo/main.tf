variable "mock" {
  description = "Mock value to be passed through"
  type        = string
}

output "mock" {
  value = var.mock
}

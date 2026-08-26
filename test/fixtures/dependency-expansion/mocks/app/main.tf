variable "web_id" {
  type = string
}

variable "api_id" {
  type = string
}

output "web_id" {
  value = var.web_id
}

output "api_id" {
  value = var.api_id
}

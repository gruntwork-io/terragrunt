variable "desired_count" {
  description = "The desired count of the ECS Service"
  type        = map(number)
}

variable "use_api_image" {
  description = "Whether to use the API image"
  type        = map(bool)
}

resource "null_resource" "services_info" {
  triggers = {
    desired_count = jsonencode(var.desired_count)
    use_api_image = jsonencode(var.use_api_image)
  }
}

output "desired_count" {
  description = "The desired count of the ECS Service"
  value       = var.desired_count
}

output "use_api_image" {
  description = "Whether to use the API image"
  value       = var.use_api_image
}

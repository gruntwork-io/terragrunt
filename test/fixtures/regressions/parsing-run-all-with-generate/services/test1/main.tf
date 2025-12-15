variable "name" {
  type = string
}

output "service_name" {
  value = var.name
}

output "service_desired_count_initial_value" {
  value = 1
}

output "docker_images" {
  value = [
    {
      repository = "123456789012.dkr.ecr.us-east-1.amazonaws.com/${var.name}-FAKE"
    }
  ]
}

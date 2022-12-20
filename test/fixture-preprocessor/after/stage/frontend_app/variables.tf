variable "frontend_config" {
  description = "The config for the frontend app"
  type = object({
    name                 = string
    replicas             = number
    docker_image_version = string
  })
}
variable "backend_config" {
  description = "The config for the backend app"
  type = object({
    name                 = string
    replicas             = number
    docker_image_version = string
  })
}
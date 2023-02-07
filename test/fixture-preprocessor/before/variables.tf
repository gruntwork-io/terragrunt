variable "vpc_config" {
  description = "The config for the VPC"
  type = object({
    name             = string
    cidr_block       = string
    num_nat_gateways = number
  })
}

variable "mysql_config" {
  description = "The config for MySQL"
  type = object({
    name                              = string
    version                           = string
    instance_class                    = string
    db_credentials_secrets_manager_id = string
  })
}

variable "frontend_config" {
  description = "The config for the frontend app"
  type = object({
    name                 = string
    replicas             = number
    docker_image_version = string
  })
}

variable "backend_config" {
  description = "The config for the backend app"
  type = object({
    name                 = string
    replicas             = number
    docker_image_version = string
  })
}
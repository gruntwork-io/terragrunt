variable "service_name" {
  description = "The name of the service"
  type        = string
}

variable "service_replicas" {
  description = "How many replicas of the service to deploy"
  type        = number
}

variable "vpc_id" {
  description = "The ID of the VPC to deploy int"
  type        = string
}

variable "docker_image" {
  description = "The Docker image to deploy"
  type        = string
}

variable "db_address" {
  description = "The address of the DB to talk to"
  type        = string
}
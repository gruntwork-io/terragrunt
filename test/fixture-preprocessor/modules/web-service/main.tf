# Use a random resource as a mock service
resource "random_string" "mock_service" {
  keepers = {
    service_name     = var.service_name
    service_replicas = var.service_replicas
    vpc_id           = var.vpc_id
    db_address       = var.db_address
    docker_image     = var.docker_image
  }

  length  = 8
  numeric = false
  special = false
  upper   = false
}

locals {
  service_id = random_string.mock_service.id
}
data "terraform_remote_state" "vpc" {
  backend = "local"
  config = {
    path = "${path.module}/../vpc/prod/vpc/terraform.tfstate"
  }
}

data "terraform_remote_state" "mysql" {
  backend = "local"
  config = {
    path = "${path.module}/../mysql/prod/mysql/terraform.tfstate"
  }
}

module "backend_app" {
  source = "../../../modules/web-service"

  service_name     = var.backend_config.name
  service_replicas = var.backend_config.replicas
  docker_image     = "gruntwork/backend-app:${var.backend_config.docker_image_version}"
  db_address       = data.terraform_remote_state.mysql.outputs.__module__.address
  vpc_id           = data.terraform_remote_state.vpc.outputs.__module__.vpc_id
}


data "terraform_remote_state" "vpc" {
  backend = "local"

  config = {
    path = "${path.module}/../vpc/vpc/terraform.tfstate"
  }
}

data "terraform_remote_state" "mysql" {
  backend = "local"

  config = {
    path = "${path.module}/../mysql/mysql/terraform.tfstate"
  }
}

module "frontend_app" {
  source = "../../../fixture-preprocessor/modules/web-service"

  service_name     = var.frontend_config.name
  service_replicas = var.frontend_config.replicas
  docker_image     = "gruntwork/frontend-app:${var.frontend_config.docker_image_version}"
  db_address       = data.terraform_remote_state.mysql.outputs.__module__.address
  vpc_id           = data.terraform_remote_state.vpc.outputs.__module__.vpc_id
}
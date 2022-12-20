module "vpc" {
  source = "../modules/vpc"

  name             = var.vpc_config.name
  cidr_block       = var.vpc_config.cidr_block
  num_nat_gateways = var.vpc_config.num_nat_gateways
}

module "mysql" {
  source = "../modules/rds"

  engine                            = "mysql"
  engine_version                    = var.mysql_config.version
  db_name                           = var.mysql_config.name
  instance_class                    = var.mysql_config.instance_class
  db_credentials_secrets_manager_id = var.mysql_config.db_credentials_secrets_manager_id
  vpc_id                            = module.vpc.vpc_id
}

module "frontend_app" {
  source = "../modules/web-service"

  service_name     = var.frontend_config.name
  service_replicas = var.frontend_config.replicas
  docker_image     = "gruntwork/frontend-app:${var.frontend_config.docker_image_version}"
  db_address       = module.mysql.address
  vpc_id           = module.vpc.vpc_id
}

module "backend_app" {
  source = "../modules/web-service"

  service_name     = var.backend_config.name
  service_replicas = var.backend_config.replicas
  docker_image     = "gruntwork/backend-app:${var.backend_config.docker_image_version}"
  db_address       = module.mysql.address
  vpc_id           = module.vpc.vpc_id
}


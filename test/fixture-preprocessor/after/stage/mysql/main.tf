data "terraform_remote_state" "vpc" {
  backend = "local"

  config = {
    path = "${path.module}/../vpc/stage/vpc/terraform.tfstate"
  }
}

module "mysql" {
  source = "../../../modules/rds"

  engine                            = "mysql"
  engine_version                    = var.mysql_config.version
  db_name                           = var.mysql_config.name
  instance_class                    = var.mysql_config.instance_class
  db_credentials_secrets_manager_id = var.mysql_config.db_credentials_secrets_manager_id
  vpc_id                            = data.terraform_remote_state.vpc.outputs.__module__.vpc_id
}
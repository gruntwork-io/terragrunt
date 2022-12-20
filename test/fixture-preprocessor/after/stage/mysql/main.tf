locals {
  // TODO: we need type constraints here. Could create a module for this?
  workspace_configurations = {
    default = {
      num_nat_gateways       = 1
      mysql_engine_version   = "8.0"
      mysql_instance_class   = "db.t3.micro"
      frontend_image_version = "v1.4.1"
      backend_image_version  = "v2.4.0"
    }

    dev = {
      cidr_block             = "10.0.0.0/16"
      frontend_image_version = "v1.4.2"
      frontend_replicas      = 1
      backend_image_version  = "v3.0.0"
      backend_replicas       = 1
    }

    stage = {
      cidr_block            = "10.2.0.0/16"
      backend_image_version = "v3.0.0"
      backend_replicas      = 2
    }

    prod = {
      num_nat_gateways     = 3
      cidr_block           = "10.4.0.0/16"
      mysql_instance_class = "db.m4.large"
      frontend_replicas    = 3
      backend_replicas     = 5
    }
  }

  workspace_config = merge(local.workspace_configurations["default"], local.workspace_configurations[local.current_workspace])
}

locals {
  current_workspace = "stage"
}

data "terraform_remote_state" "vpc" {
  backend = "local"
  config = {
    path = "${path.module}/../vpc/stage/vpc/terraform.tfstate"
  }
}

module "mysql" {
  source = "../../../modules/rds"

  db_name        = "mysql-${local.current_workspace}"
  engine         = "mysql"
  engine_version = lookup(local.workspace_config, "mysql_engine_version")
  instance_class = lookup(local.workspace_config, "mysql_instance_class")
  db_username    = var.db_username
  db_password    = var.db_password
  vpc_id         = data.terraform_remote_state.vpc.outputs.__module__.vpc_id
}

vpc_config = {
  name             = "vpc-prod"
  cidr_block       = "10.4.0.0/16"
  num_nat_gateways = 3
}

mysql_config = {
  name                              = "mysql-prod"
  version                           = "8.0"
  instance_class                    = "db.m4.large"
  db_credentials_secrets_manager_id = "mysql-credentials"
}

frontend_config = {
  name                 = "frontend-prod"
  replicas             = 3
  docker_image_version = "v2.4.1"
}

backend_config = {
  name                 = "backend-prod"
  replicas             = 4
  docker_image_version = "v1.3.4"
}
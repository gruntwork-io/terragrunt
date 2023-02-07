# Use a random resource as a mock RDS DB
resource "random_string" "mock_rds" {
  keepers = {
    db_name                           = var.db_name
    engine                            = var.engine
    engine_version                    = var.engine_version
    instance_class                    = var.instance_class
    vpc_id                            = var.vpc_id
    db_credentials_secrets_manager_id = var.db_credentials_secrets_manager_id
  }

  length  = 8
  numeric = false
  special = false
  upper   = false
}

locals {
  rds_id = "rds-${random_string.mock_rds.id}"
}
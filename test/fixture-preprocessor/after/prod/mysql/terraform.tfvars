mysql_config = {
  name                              = "mysql-prod"
  version                           = "8.0"
  instance_class                    = "db.m4.large"
  db_credentials_secrets_manager_id = "mysql-credentials"
}
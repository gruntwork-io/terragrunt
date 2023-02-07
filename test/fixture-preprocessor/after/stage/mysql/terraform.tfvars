mysql_config = {
  name                              = "mysql-stage"
  version                           = "8.0"
  instance_class                    = "db.t3.micro"
  db_credentials_secrets_manager_id = "mysql-credentials"
}
variable "mysql_config" {
  description = "The config for MySQL"
  type = object({
    name                              = string
    version                           = string
    instance_class                    = string
    db_credentials_secrets_manager_id = string
  })
}
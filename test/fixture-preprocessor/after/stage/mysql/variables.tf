variable "db_username" {
  description = "The username for the master user of the DB"
  type        = string
}

variable "db_password" {
  description = "The password for the master user of the DB"
  type        = string
  sensitive   = true
}
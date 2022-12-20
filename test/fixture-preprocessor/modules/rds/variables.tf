variable "db_name" {
  description = "The name of the database"
  type        = string
}

variable "engine" {
  description = "The DB engine to use. Must be one of: mysql, postgres, aurora"
  type        = string
  validation {
    condition     = contains(["mysql", "postgres", "aurora"], var.engine)
    error_message = "Unsupported engine"
  }
}

variable "engine_version" {
  description = "The version of the DB engine to use."
  type        = string
}

variable "instance_class" {
  description = "The instance type to use (e.g., db.t3.micro)"
  type        = string
}

variable "vpc_id" {
  description = "The VPC to deploy into"
  type        = string
}

variable "db_credentials_secrets_manager_id" {
  description = "The ID of a secret to read from AWS Secrets Manager to fetch the database credentials (username, password)"
  type        = string
}

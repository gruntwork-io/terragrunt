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

variable "db_username" {
  description = "The username for the master user of the DB"
  type        = string
}

variable "db_password" {
  description = "The password for the master user of the DB"
  type        = string
  sensitive   = true
}

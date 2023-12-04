variable "project_name" {
  type        = string
  description = "Project name"
}

variable "vpc" {
  type        = string
  description = "VPC to be used"
  default     = "default-vpc"
}

variable "replica_count" {
  type    = number
  default = 666
}

variable "enabled" {
  description = "Enable or disable the module"
  type        = bool
  default     = true
}

variable "open_port" {
  type        = number
  description = "Port to open"
}

variable "enable_backups" {
  type = bool
}

variable "users" {
  type        = list(string)
  description = "List of users"
}

variable "policy_map" {
  type        = map(string)
  description = "Map of policies"
}

variable "test_1" {}

variable "test_2" {
  default = {
    x = 1
  }
}

variable "test_3" {
  type        = number
  default     = 666
  description = "description test 3"
}

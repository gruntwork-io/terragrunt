variable "project_name" {
  type        = string
  description = "Project name"

}

variable "open_port" {
  type        = number
  description = "Port to open"
}

variable "enable_backups" {
  type = bool
}

variable "no_type_value_var" {}


variable "default_var" {
  default = {
    x = 1
  }
}

variable "number_default" {
  type        = number
  default     = 42
  description = "number variable with default"
}

variable "object_var" {
  type = object({
    str = string
    num = number
  })

  default = {
    str = "default"
    num = 42
  }
}

variable "map_var" {
  type = map(string)
  default = {
    key = "value42"
  }
}

variable "list_var" {
  type    = list(number)
  default = [1, 2, 3]
}

variable "enabled" {
  description = "Enable or disable the module"
  type        = bool
  default     = true
}

variable "vpc" {
  type        = string
  description = "VPC to be used"
  default     = "default-vpc"

}

terraform {
  required_providers {
    local = {
      source  = "registry.opentofu.org/hashicorp/local"
      version = "2.6.1"
    }
  }
}


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

resource "local_file" "config" {
  content  = "${var.project_name} vpc: ${var.vpc} replica_count: ${var.replica_count}"
  filename = "config.txt"
}

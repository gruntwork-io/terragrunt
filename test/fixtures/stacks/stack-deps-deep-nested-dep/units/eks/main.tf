variable "vpc_id" {
  type    = string
  default = ""
}

variable "subnet_ids" {
  type    = list(string)
  default = []
}

output "cluster_vpc" {
  value = var.vpc_id
}

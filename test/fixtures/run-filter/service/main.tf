variable "vpc_id" {
  type = string
}

variable "db_id" {
  type = string
}

variable "cache_id" {
  type = string
}

resource "null_resource" "service" {
  triggers = {
    name     = "service"
    vpc_id   = var.vpc_id
    db_id    = var.db_id
    cache_id = var.cache_id
  }
}

output "service_id" {
  value = "service-${var.vpc_id}-${var.db_id}-${var.cache_id}"
}


variable "vpc_id" {
  type = string
}

resource "null_resource" "db" {
  triggers = {
    name   = "db"
    vpc_id = var.vpc_id
  }
}

output "db_id" {
  value = "db-${var.vpc_id}"
}


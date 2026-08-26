variable "db_id" {
  type = string
}

variable "dns_id" {
  type = string
}

resource "null_resource" "app" {
  triggers = {
    name   = "app"
    db_id  = var.db_id
    dns_id = var.dns_id
  }
}

output "app_id" {
  value = "app-${var.db_id}-${var.dns_id}"
}

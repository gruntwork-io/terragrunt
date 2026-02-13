variable "from_root" {
  type    = string
  default = ""
}

variable "name" {
  type = string
}

variable "vpc_id" {
  type    = string
  default = ""
}

variable "app1_id" {
  type    = string
  default = ""
}

resource "null_resource" "test" {
  triggers = {
    name      = var.name
    from_root = var.from_root
    vpc_id    = var.vpc_id
    app1_id   = var.app1_id
  }
}

output "name" {
  value = var.name
}

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
  }
}

output "name" {
  value = var.name
}

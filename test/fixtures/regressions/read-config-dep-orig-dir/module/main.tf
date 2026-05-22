variable "unit_name" {
  type    = string
  default = ""
}

variable "from_dep" {
  type    = string
  default = ""
}

resource "terraform_data" "this" {
  input = var.unit_name
}

output "name" {
  value = var.unit_name
}

output "dep_value" {
  value = var.from_dep
}

variable "root_value" {
  type    = string
  default = ""
}

variable "dep_output_value" {
  type    = string
  default = ""
}

output "combined" {
  value = "${var.root_value}-${var.dep_output_value}"
}

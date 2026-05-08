variable "sub_id" {
  type = string
}

variable "rgs_name" {
  type = string
}

output "value" {
  value = "${var.sub_id}/${var.rgs_name}"
}

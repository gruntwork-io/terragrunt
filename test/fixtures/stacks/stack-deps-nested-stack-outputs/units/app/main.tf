variable "vpc_id" {
  type = string
}

variable "subnet_id" {
  type = string
}

output "received" {
  value = "${var.vpc_id}/${var.subnet_id}"
}

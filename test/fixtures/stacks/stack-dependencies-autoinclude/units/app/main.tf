variable "env" {}
variable "vpc_id" {}

output "result" {
  value = "${var.env}-${var.vpc_id}"
}

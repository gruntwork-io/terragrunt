variable "name" {}

output "vpc_id" {
  value = "vpc-${var.name}-asdf1234"
}

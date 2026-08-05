variable "vpc_id" {
  type = string
}

variable "cidr" {
  type = string
}

output "subnet_id" {
  value = "subnet-12345"
}

variable "availability_zones" {
  type    = list(string)
  default = []
}

output "vpc_id" {
  value = "vpc-real-output"
}

output "private_subnet_ids" {
  value = ["subnet-real-a", "subnet-real-b"]
}

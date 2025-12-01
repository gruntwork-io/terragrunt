resource "null_resource" "vpc" {
  triggers = {
    name = "vpc"
  }
}

output "vpc_id" {
  value = "vpc-12345"
}


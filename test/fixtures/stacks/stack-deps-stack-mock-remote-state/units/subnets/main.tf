terraform {
  backend "s3" {}
}

output "subnet_id" {
  value = "real-subnet-id"
}

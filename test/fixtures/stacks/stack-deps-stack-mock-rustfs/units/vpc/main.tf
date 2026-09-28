terraform {
  backend "s3" {}
}

output "id" {
  value = "real-vpc-id"
}

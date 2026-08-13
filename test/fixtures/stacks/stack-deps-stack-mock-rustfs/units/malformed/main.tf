terraform {
  backend "s3" {}
}

output "id" {
  value = "real-malformed-id"
}

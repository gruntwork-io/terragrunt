terraform {
  backend "s3" {}
}

output "text" {
  value = "app2 output"
}

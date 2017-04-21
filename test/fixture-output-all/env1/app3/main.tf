terraform {
  backend "s3" {}
}

output "text" {
  value = "app3 output"
}

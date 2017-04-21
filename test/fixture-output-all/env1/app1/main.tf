terraform {
  backend "s3" {}
}

output "text" {
  value = "app1 output"
}

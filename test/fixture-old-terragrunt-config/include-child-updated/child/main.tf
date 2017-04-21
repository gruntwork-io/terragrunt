terraform {
  backend "s3" {}
}

output "hello" {
  value = "Hello, World"
}
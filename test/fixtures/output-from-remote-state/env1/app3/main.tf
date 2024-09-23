terraform {
  backend "s3" {}
}

output "app3_text" {
  value = "app3 output"
}

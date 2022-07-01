terraform {
  backend "s3" {}
}

output "app1_text" {
  value = "app1 output"
}

terraform {
  backend "s3" {}
}

output "app2_text" {
  value = "app2 output"
}

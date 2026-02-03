terraform {
  backend "gcs" {}
}

output "app3_text" {
  value = "app3 output"
}

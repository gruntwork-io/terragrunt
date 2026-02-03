terraform {
  backend "gcs" {}
}

output "app1_text" {
  value = "app1 output"
}

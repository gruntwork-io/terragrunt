terraform {
  backend "gcs" {}
}

output "app1_text" {
  value = var.app1_text
}

output "app2_text" {
  value = "app2 output"
}

output "app3_text" {
  value = var.app3_text
}

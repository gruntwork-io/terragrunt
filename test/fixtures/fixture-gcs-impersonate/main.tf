terraform {
  backend "gcs" {}
}

output "value" {
  value = "42"
}

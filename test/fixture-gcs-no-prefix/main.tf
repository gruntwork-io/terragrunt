terraform {
  backend "gcs" {}
}

output "result" {
  value = "42"
}

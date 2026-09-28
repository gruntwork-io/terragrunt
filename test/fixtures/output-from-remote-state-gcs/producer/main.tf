terraform {
  backend "gcs" {}
}

output "producer_value" {
  value = "from-gcs-state"
}

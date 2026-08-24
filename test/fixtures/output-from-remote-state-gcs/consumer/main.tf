terraform {
  backend "gcs" {}
}

variable "producer_value" {
  type = string
}

output "consumer_value" {
  value = var.producer_value
}

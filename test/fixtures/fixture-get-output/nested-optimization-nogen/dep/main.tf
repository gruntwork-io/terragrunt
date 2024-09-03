terraform {
  backend "s3" {}
}

variable "input" {}
output "output" {
  value = "No, ${var.input}"
}

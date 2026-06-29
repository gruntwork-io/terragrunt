variable "ns" {
  type = string
}

resource "terraform_data" "a" {}

output "echo" {
  value = var.ns
}

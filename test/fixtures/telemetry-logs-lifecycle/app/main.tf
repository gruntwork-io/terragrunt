variable "value" {
  type = string
}

resource "terraform_data" "app" {
  input = var.value
}

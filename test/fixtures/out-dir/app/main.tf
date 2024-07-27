variable "input_value" {}

output "output_value" {
  value = var.input_value
}

resource "local_file" "app_file" {
  content  = "app file"
  filename = "${path.module}/app_file.txt"
}
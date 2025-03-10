variable "app1_output" {
  type = string
}

resource "local_file" "test" {
  content  = var.app1_output
  filename = "${path.module}/test.txt"
}

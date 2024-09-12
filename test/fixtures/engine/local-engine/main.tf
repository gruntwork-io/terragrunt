
variable "value" {}

resource "local_file" "test" {
  content  = "test ${var.value}"
  filename = "${path.module}/test.txt"
}

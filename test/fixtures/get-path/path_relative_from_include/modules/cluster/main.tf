variable "some_input" {}

output "some_output" {
  value = "${var.some_input} else"
}

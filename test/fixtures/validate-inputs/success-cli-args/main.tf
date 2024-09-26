variable "input" {}
variable "other_input" {}

output "output" {
  value = "${var.input} ${var.other_input}"
}

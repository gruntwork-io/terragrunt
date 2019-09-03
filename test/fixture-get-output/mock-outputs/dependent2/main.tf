variable "the_answer" {}

output "fake" {
  value = "never ${var.the_answer}"
}

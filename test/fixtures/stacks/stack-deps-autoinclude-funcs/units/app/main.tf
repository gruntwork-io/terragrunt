variable "data_value" {
  type = string
}

variable "greeting" {
  type = string
}

output "out" {
  value = "${var.data_value}:${var.greeting}"
}

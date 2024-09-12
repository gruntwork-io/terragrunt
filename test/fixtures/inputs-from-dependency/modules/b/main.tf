variable "baz" {
  type    = string
  default = "empty"
}

output "baz" {
  value = var.baz
}

variable "foo" {
  type    = string
  default = "empty"
}

output "foo" {
  value = var.foo
}

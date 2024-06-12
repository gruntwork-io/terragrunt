variable "foo" {}

variable "foo-app3" {}

output "foo-app2" {
  value = var.foo
}

output "foo-app3" {
  value = var.foo-app3
}

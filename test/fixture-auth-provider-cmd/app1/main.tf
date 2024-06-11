variable "foo" {}

variable "foo-app2" {}

variable "foo-app3" {}

output "foo-app1" {
  value = var.foo
}

output "foo-app2" {
  value = var.foo-app2
}

output "foo-app3" {
  value = var.foo-app3
}

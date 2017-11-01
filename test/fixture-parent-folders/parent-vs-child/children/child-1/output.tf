variable "foo_parent" {}
variable "foo_child" {}
variable "foo_from_include" {}
variable "foo_to_include" {}
variable "bar_parent" {}
variable "bar_child" {}
variable "bar_from_include" {}
variable "bar_to_include" {}

output "parent" {
  value = {
    parent       = "${var.foo_parent}"
    child        = "${var.foo_child}"
    from_include = "${var.foo_from_include}"
    to_include   = "${var.foo_to_include}"
  }
}

output "child" {
  value = {
    parent       = "${var.bar_parent}"
    child        = "${var.bar_child}"
    from_include = "${var.bar_from_include}"
    to_include   = "${var.bar_to_include}"
  }
}

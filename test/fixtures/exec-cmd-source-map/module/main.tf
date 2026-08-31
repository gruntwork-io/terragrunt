variable "name" {}

output "mapped_module_id" {
  value = "mapped-${var.name}"
}

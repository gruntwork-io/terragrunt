resource "null_resource" "c" {}

output "c" {
  value = null_resource.c.id
}

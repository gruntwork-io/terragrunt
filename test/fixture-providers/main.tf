resource "null_resource" "b" {}

output "b" {
  value = null_resource.b.id
}

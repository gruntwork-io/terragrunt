resource "null_resource" "a" {}

output "a" {
  value = null_resource.a.id
}

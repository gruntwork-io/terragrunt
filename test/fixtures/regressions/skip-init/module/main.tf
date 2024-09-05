resource "null_resource" "foo" {}

output "foo" {
  value = null_resource.foo.id
}

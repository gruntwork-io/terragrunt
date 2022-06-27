resource "null_resource" "main" {}

output "id" {
  value = null_resource.main.id
}

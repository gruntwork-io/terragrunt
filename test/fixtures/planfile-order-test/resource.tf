resource "null_resource" "test-resources" {
  count = var.resource_count
}

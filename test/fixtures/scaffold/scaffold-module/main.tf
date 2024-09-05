
resource "local_file" "config" {
  content  = "${var.project_name} vpc: ${var.vpc} replica_count: ${var.replica_count}"
  filename = "config.txt"
}

variable "content" {}

resource "local_file" "file" {
  content  = "content: ${var.content}"
  filename = "${path.module}/cluster_name.txt"
}
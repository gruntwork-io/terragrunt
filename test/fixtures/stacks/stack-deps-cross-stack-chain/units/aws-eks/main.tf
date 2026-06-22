variable "vpc_id" {
  type = string
}

resource "local_file" "marker" {
  content  = "eks on ${var.vpc_id}"
  filename = "${path.module}/marker.txt"
}

output "cluster_id" {
  value = "cluster-on-${var.vpc_id}"
}

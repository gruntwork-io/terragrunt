variable "cluster_id" {
  type = string
}

resource "local_file" "marker" {
  content  = "karpenter on ${var.cluster_id}"
  filename = "${path.module}/marker.txt"
}

output "status" {
  value = "karpenter-on-${var.cluster_id}"
}

variable "cluster_id" {
  type    = string
  default = ""
}

output "bootstrap_status" {
  value = "rancher-bootstrap-${var.cluster_id}"
}

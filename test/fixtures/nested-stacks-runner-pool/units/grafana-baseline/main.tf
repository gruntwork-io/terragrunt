variable "cluster_id" {
  type    = string
  default = ""
}

output "grafana_status" {
  value = "grafana-baseline-${var.cluster_id}"
}

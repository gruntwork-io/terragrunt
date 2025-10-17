variable "cluster_id" {
  type    = string
  default = ""
}

output "baseline_status" {
  value = "eks-baseline-${var.cluster_id}"
}

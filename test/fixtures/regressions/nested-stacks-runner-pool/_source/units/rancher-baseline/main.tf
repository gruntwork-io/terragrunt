variable "bootstrap_status" {
  type    = string
  default = ""
}

output "baseline_status" {
  value = "rancher-baseline-${var.bootstrap_status}"
}

variable "vpc_id" {
  type    = string
  default = ""
}

output "router_id" {
  value = "tailscale-router-${var.vpc_id}"
}

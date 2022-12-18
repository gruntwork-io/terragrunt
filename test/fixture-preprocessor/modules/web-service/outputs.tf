output "service_address" {
  value = "${var.service_name}-${local.service_id}.us-east-1.elb.amazonaws.com"
}
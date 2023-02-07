output "db_id" {
  value = local.rds_id
}

output "address" {
  value = "${var.db_name}.${local.rds_id}.us-east-1.rds.amazonaws.com"
}
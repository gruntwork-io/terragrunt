output "vpc_id" {
  value = module.vpc.vpc_id
}

output "mysql_id" {
  value = module.mysql.db_id
}

output "mysql_address" {
  value = module.mysql.address
}

output "frontend_app_address" {
  value = module.frontend_app.service_address
}

output "backend_app_address" {
  value = module.backend_app.service_address
}


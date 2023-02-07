output "frontend_app_address" {
  value = module.frontend_app.service_address
}

output "__module__" {
  description = "This output is added by Terragrunt so that modules that depend on each other can read all the info they need from each other's state files using this output variable and the terraform_remote_state data source."
  value       = module.frontend_app
}
locals {
  app1_path = "../app1"
  app2_path = "../app2"
}

terragrunt_output "app1" {
  config_path = local.app1_path
}

terragrunt_output "app2" {
  config_path = local.app2_path
}

inputs = {
  x = terragrunt_output.app1.x
  y = terragrunt_output.app2.y
}

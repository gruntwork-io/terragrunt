locals {
  environment_vars = read_terragrunt_config(find_in_parent_folders("environment.hcl")).locals
}

remote_state {
  backend = "local"
  config = {
    path = "${path_relative_to_include()}/terraform.tfstate"
  }
}

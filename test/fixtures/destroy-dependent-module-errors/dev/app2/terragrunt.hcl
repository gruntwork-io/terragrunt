locals {
    env_config  = read_terragrunt_config(find_in_parent_folders("env.hcl"))
}

dependency "app1" {
  config_path = "../app1"
}

inputs = {
    data  = dependency.app1.outputs.value
}
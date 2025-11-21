locals {
    env_config  = read_terragrunt_config(find_in_parent_folders("env.hcl"))
}

inputs = {
    data  = local.env_config.locals.env
}
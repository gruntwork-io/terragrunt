# A unit whose generated terragrunt.hcl uses locals
# driven by read_terragrunt_config(find_in_parent_folders("region.hcl")) and
# find_in_parent_folders, passes values from local.* and values.*, alongside a
# unit that carries an autoinclude with a dependency. stack generate must succeed.
locals {
  shared_env = "prod"
}

unit "account" {
  source = "../catalog/units/account"
  path   = "account"

  values = {
    account = values.account
    env     = local.shared_env
  }
}

unit "roles" {
  source = "../catalog/units/roles"
  path   = "roles"

  values = {
    roles = values.roles
    env   = local.shared_env
  }

  autoinclude {
    dependency "account" {
      config_path = unit.account.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-account"
      }
    }

    inputs = {
      val = dependency.account.outputs.val
    }
  }
}

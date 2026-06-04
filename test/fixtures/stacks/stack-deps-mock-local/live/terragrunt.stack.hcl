locals {
  account = {
    name = "my-account"
  }
}

unit "account" {
  source = "${get_repo_root()}/units/account"
  path   = "account"
}

unit "iam" {
  source = "${get_repo_root()}/units/iam"
  path   = "iam"

  autoinclude {
    dependency "account" {
      config_path = unit.account.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        name     = local.account.name                # resolvable at generate time -> must resolve
        deferred = dependency.account.outputs.name    # runtime-only -> must stay literal
      }
    }

    inputs = {
      account_name = try(dependency.account.outputs.name, local.account.name)
    }
  }
}

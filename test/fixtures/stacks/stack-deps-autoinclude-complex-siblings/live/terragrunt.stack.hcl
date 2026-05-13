// Fixture: combines terragrunt function calls, local.* references, and values.* references in unrelated units alongside an autoinclude block on a single unit.

locals {
  shared_region = "us-east-1"
}

unit "account" {
  source = "${get_terragrunt_dir()}/../catalog/units/account"
  path   = "account"
  values = {
    account = values.account
    region  = local.shared_region
  }
}

unit "idps" {
  source = "${get_terragrunt_dir()}/../catalog/units/idp"
  path   = "idps"
  values = {
    identityProviders = values.idps
    region            = local.shared_region
  }
}

unit "roles" {
  source = "${get_terragrunt_dir()}/../catalog/units/roles"
  path   = "roles"
  values = {
    roles  = values.roles
    region = local.shared_region
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

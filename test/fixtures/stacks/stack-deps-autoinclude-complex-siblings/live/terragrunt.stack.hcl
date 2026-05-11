// Mirrors the HCL pattern from https://github.com/gruntwork-io/terragrunt/issues/5663#issuecomment-4373686666
// Combines every feature class that previously broke the simplified two-pass parser:
//   - terragrunt function call (get_terragrunt_dir, equivalent at parse-time to get_repo_root) in source
//   - local.* references in values
//   - values.* references in values
//   - autoinclude block on a single unit only
//
// All three failure modes must coexist with autoinclude generation succeeding.

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

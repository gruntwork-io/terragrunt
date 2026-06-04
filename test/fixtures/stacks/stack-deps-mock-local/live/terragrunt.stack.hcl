locals {
  account = {
    # A sentinel that only appears in the generated file if the local in mock_outputs is resolved.
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

  values = {
    region = "eu-west-1"
  }

  autoinclude {
    dependency "account" {
      config_path = unit.account.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        # A local is generate-time-knowable, so it must be resolved here (this is the bug under test).
        name = local.account.name
        # A unit value is resolved when the generated unit is parsed, so it must stay deferred (verbatim) and
        # then resolve from the unit's terragrunt.values.hcl when the generated unit is parsed.
        region = values.region
      }
    }
  }
}

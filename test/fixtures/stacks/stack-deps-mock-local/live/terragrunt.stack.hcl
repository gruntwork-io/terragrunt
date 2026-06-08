locals {
  account = {
    # Stack-level locals are resolved in the autoinclude at generate time.
    name   = "my-account"
    region = "eu-west-1"
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
        # Stack-level locals are generate-time-knowable, so they resolve to literals here.
        name   = local.account.name
        region = local.account.region
      }
    }
  }
}

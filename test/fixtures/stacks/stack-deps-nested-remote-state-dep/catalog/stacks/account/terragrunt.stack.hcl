unit "account" {
  source = "${get_repo_root()}/catalog/units/account"
  path   = "account_hcl"
}

unit "roles" {
  source = "${get_repo_root()}/catalog/units/roles"
  path   = "roles_hcl"

  autoinclude {
    dependency "account" {
      config_path = unit.account.path

      mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
      mock_outputs = {
        id   = "mock-id"
        name = "mock-account"
      }
    }
  }
}

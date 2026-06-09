unit "x" {
  source = "../catalog/x"
  path   = "x"
}

unit "y" {
  source = "../catalog/y"
  path   = "y"

  autoinclude {
    dependency "x" {
      config_path = unit.x.path

      mock_outputs_allowed_terraform_commands = ["init", "plan", "validate"]
      mock_outputs = {
        from_autoinclude = "autoval"
        common           = "autoinclude-common"
      }
    }
  }
}

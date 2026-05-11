// Parent stack: depends on the entire `networking` stack via autoinclude.
// The catalog stack file contains a terragrunt function call in unit.source which
// is copied verbatim into the generated nested stack file. `run --all` discovery
// must walk that generated file successfully (regression for #5663 comment 4407441298).

stack "networking" {
  source = "../catalog/stacks/networking"
  path   = "networking"
}

unit "app" {
  source = "../catalog/units/vpc"
  path   = "app"

  autoinclude {
    dependency "networking" {
      config_path = stack.networking.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply", "destroy"]
      mock_outputs = {
        vpc_id = "mock-vpc-id"
      }
    }
  }
}

// Fixture: parent stack depends on the entire `networking` stack via autoinclude; the catalog stack file contains a function call in unit.source that must not block run --all discovery.

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

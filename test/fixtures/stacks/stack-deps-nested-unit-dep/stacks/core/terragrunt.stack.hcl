unit "data" {
  source = "${get_repo_root()}/units/data"
  path   = "data"
}

unit "vpc" {
  source = "${get_repo_root()}/units/vpc"
  path   = "vpc"

  autoinclude {
    dependency "data" {
      config_path = unit.data.path

      mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "apply"]
      mock_outputs = {
        availability_zones = ["region-a", "region-b", "region-c"]
      }
    }

    inputs = {
      availability_zones = dependency.data.outputs.availability_zones
    }
  }
}

// Regression fixture for issue #5980: stack file uses an HCL function in path that
// only the production parser has registered (the simplified two-pass autoinclude
// parser uses nil eval context). With an autoinclude block declared, this must fail
// loudly during stack generate instead of silently skipping autoinclude generation.

unit "vpc" {
  source = "../catalog/units/vpc"
  path   = format("%s", "vpc")
}

unit "subnet" {
  source = "../catalog/units/subnet"
  path   = "subnet"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan", "apply"]
      mock_outputs = {
        id = "mock-id"
      }
    }

    inputs = {
      vpc_id = dependency.vpc.outputs.id
    }
  }
}

locals {
  prefix = "pre"
}

unit "vpc" {
  source = "../units/vpc"
  path   = "vpc"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "vpc" {
      config_path = unit.vpc.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        id = "mock-vpc-id"
        # An interpolated key inside a dependency block attribute resolves at generate time too.
        "${local.prefix}_mock" = "mock-extra"
      }
    }

    inputs = {
      # The whole object is generate-time-knowable: the interpolated key and the literal value both resolve here.
      pure_obj = { "${local.prefix}_key" = "literal" }
      # The value defers to dependency.*, but the interpolated key must still resolve at generate time and not leak a stack-scoped reference into the generated unit.
      mixed_obj = { "${local.prefix}_key" = dependency.vpc.outputs.id }
    }
  }
}

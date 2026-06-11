# Cross-stack dependency where the child stack's terragrunt.stack.hcl reads
# values.* in its locals. The parent passes values to the stack block, so stack
# generation writes a terragrunt.values.hcl next to the generated child stack
# file; run-queue expansion of the stack-dir dependency must load it.

stack "network" {
  source = "../stacks/network"
  path   = "network"

  values = {
    env = "dev"
  }
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "network" {
      config_path = stack.network.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        vpc = {
          vpc_id = "mock-vpc"
        }
      }
    }

    inputs = {
      vpc_id = dependency.network.outputs.vpc.vpc_id
    }
  }
}

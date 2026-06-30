# Cross-stack dependency: unit "app" depends on the entire "network" stack.
# Parent uses relative paths (required for autoinclude two-pass parser).
# Nested stack uses get_repo_root() (required for go-getter in production).

stack "network" {
  source = "../stacks/network"
  path   = "network"
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

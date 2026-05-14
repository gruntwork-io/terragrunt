# Tests stack.<name>.<nested_stack_name>.path resolution.
# app depends on the "deep" nested stack inside "infra" stack.

stack "infra" {
  source = "../stacks/infra"
  path   = "infra"
}

unit "app" {
  source = "../units/app"
  path   = "app"

  autoinclude {
    dependency "deep" {
      config_path = stack.infra.deep.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-db"
      }
    }

    inputs = {
      val = dependency.deep.outputs.val
    }
  }
}

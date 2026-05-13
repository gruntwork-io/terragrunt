// Fixture: stack autoinclude `values = {...}` computed from a dependency on a sibling stack's unit output must reach the nested stack's units as values.<key>.

stack "stack_w_outputs" {
  source = "../catalog/stacks/stack-w-outputs"
  path   = "stack-w-outputs"
}

stack "stack_w_values" {
  source = "../catalog/stacks/stack-w-values"
  path   = "stack-w-values"

  autoinclude {
    dependency "unit_w_outputs" {
      config_path = stack.stack_w_outputs.unit_w_outputs.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-val"
      }
    }

    values = {
      val = dependency.unit_w_outputs.outputs.val
    }
  }
}

# Docs example: Dependencies on units within a nested stack
# stack.stack_w_outputs.unit_w_outputs.path resolves to the nested unit path

stack "stack_w_outputs" {
  source = "../catalog/stacks/stack-w-outputs"
  path   = "stack-w-outputs"
}

unit "unit_w_inputs" {
  source = "../catalog/units/unit-w-inputs"
  path   = "unit-w-inputs"

  autoinclude {
    dependency "unit_w_outputs" {
      config_path = stack.stack_w_outputs.unit_w_outputs.path

      mock_outputs_allowed_terraform_commands = ["plan"]
      mock_outputs = {
        val = "fake-val"
      }
    }

    inputs = {
      val = dependency.unit_w_outputs.outputs.val
    }
  }
}

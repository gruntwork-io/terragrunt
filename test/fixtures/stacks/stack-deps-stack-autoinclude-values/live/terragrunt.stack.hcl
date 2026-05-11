// Regression for https://github.com/gruntwork-io/terragrunt/issues/5663#issuecomment-4406850040
//
// stack "stack_w_values" declares an autoinclude with a `values { … }` block whose attributes are
// computed from a dependency on a sibling stack's unit output. The expected behavior is that the
// nested stack's units (under .terragrunt-stack/stack-w-values/.terragrunt-stack/unit_w_inputs/) see
// those values as `values.<key>` in their terragrunt.hcl, so the unit's `inputs = { val = values.val }`
// resolves to the real upstream output ("real-upstream-output").

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

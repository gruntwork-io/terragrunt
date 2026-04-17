locals {
  env = "test"
}

unit "unit_w_outputs" {
  source = "../catalog/units/unit-w-outputs"
  path   = "unit-w-outputs"
}

unit "unit_w_inputs" {
  source = "../catalog/units/unit-w-inputs"
  path   = "unit-w-inputs"

  autoinclude {
    dependency "unit_w_outputs" {
      config_path = unit.unit_w_outputs.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "fake-val"
      }
    }

    inputs = {
      val = dependency.unit_w_outputs.outputs.val
    }
  }
}

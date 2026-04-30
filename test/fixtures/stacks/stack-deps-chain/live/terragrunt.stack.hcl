# 3-level dependency chain: unit_a -> unit_b -> unit_c
# unit_c produces "from-c"
# unit_b consumes unit_c output, produces "from-b(from-c)"
# unit_a consumes unit_b output, produces "from-a(from-b(from-c))"

unit "unit_c" {
  source = "../catalog/units/unit-c"
  path   = "unit-c"
}

unit "unit_b" {
  source = "../catalog/units/unit-b"
  path   = "unit-b"

  autoinclude {
    dependency "unit_c" {
      config_path = unit.unit_c.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-c"
      }
    }

    inputs = {
      val_from_c = dependency.unit_c.outputs.val
    }
  }
}

unit "unit_a" {
  source = "../catalog/units/unit-a"
  path   = "unit-a"

  autoinclude {
    dependency "unit_b" {
      config_path = unit.unit_b.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-b"
      }
    }

    inputs = {
      val_from_b = dependency.unit_b.outputs.val
    }
  }
}

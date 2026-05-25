# Multi-level dependency tree:
#
#       A
#      / \
#     B   C
#    / \
#   D   E
#
# D, E, C are leaves (no dependencies).
# B depends on D and E.
# A depends on B and C.

unit "unit_d" {
  source = "../catalog/units/unit-d"
  path   = "unit-d"
}

unit "unit_e" {
  source = "../catalog/units/unit-e"
  path   = "unit-e"
}

unit "unit_c" {
  source = "../catalog/units/unit-c"
  path   = "unit-c"
}

unit "unit_b" {
  source = "../catalog/units/unit-b"
  path   = "unit-b"

  autoinclude {
    dependency "unit_d" {
      config_path = unit.unit_d.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-d"
      }
    }

    dependency "unit_e" {
      config_path = unit.unit_e.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-e"
      }
    }

    inputs = {
      val_from_d = dependency.unit_d.outputs.val
      val_from_e = dependency.unit_e.outputs.val
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

    dependency "unit_c" {
      config_path = unit.unit_c.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-c"
      }
    }

    inputs = {
      val_from_b = dependency.unit_b.outputs.val
      val_from_c = dependency.unit_c.outputs.val
    }
  }
}

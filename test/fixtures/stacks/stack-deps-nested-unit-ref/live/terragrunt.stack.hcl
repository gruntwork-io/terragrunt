# The consumer depends on a specific unit inside the nested core stack using the explicit nested reference
# stack.core.unit.producer.path, which resolves to the unit's full generated path (through the nested
# .terragrunt-stack directory) by construction, symmetric with unit.<name>.path and stack.<name>.path.
stack "core" {
  source = "${get_repo_root()}/stacks/core"
  path   = "core"
}

unit "consumer" {
  source = "${get_repo_root()}/units/consumer"
  path   = "consumer"

  autoinclude {
    dependency "producer" {
      config_path = stack.core.unit.producer.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        value = "mock-producer-output"
      }
    }

    inputs = {
      received = try(values.received, dependency.producer.outputs.value)
    }
  }
}

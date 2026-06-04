# The consumer unit depends on a SPECIFIC unit inside the nested core stack by drilling into it with
# ${stack.core.path}/producer. The core stack's units live under its inner .terragrunt-stack directory, so
# this config_path must resolve through that segment rather than landing at core/producer one level too high.
stack "core" {
  source = "${get_repo_root()}/stacks/core"
  path   = "core"
}

unit "consumer" {
  source = "${get_repo_root()}/units/consumer"
  path   = "consumer"

  autoinclude {
    dependency "producer" {
      config_path = "${stack.core.path}/producer"

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

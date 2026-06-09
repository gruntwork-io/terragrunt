# Same cross-level shape as stack-deps-cross-level-values, plus a sibling unit
# carrying its own autoinclude in the parent file. The sibling autoinclude used
# to break unit.producer.path resolution inside the stack block's values.
unit "producer" {
  source = "../catalog/units/producer"
  path   = "producer"
}

stack "child" {
  source = "../catalog/stacks/child"
  path   = "child"

  values = {
    producer_path = unit.producer.path
  }
}

unit "sibling" {
  source = "../catalog/units/consumer"
  path   = "sibling"

  autoinclude {
    dependency "producer" {
      config_path = unit.producer.path

      mock_outputs_allowed_terraform_commands = ["validate", "plan"]
      mock_outputs = {
        val = "mock-val"
      }
    }

    inputs = {
      val = dependency.producer.outputs.val
    }
  }
}

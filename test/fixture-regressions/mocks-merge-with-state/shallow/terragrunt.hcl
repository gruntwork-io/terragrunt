dependency "module" {
  config_path = "../module"
  mock_outputs_merge_strategy_with_state = "shallow"
}

inputs = {
  field = dependency.module.outputs.data.field
}

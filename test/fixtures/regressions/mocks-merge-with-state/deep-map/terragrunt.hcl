dependency "module" {
  config_path = "../module"
  mock_outputs_merge_strategy_with_state = "deep_map_only"
}

inputs = {
  field = dependency.module.outputs.data.field
}

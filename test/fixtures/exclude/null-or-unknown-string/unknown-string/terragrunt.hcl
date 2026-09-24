terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    flag = "false"
  }
}

# Discovery can't read dependency outputs, so it skips this block with a warning
exclude {
  if      = tostring(dependency.dep.outputs.flag)
  actions = ["all"]
}

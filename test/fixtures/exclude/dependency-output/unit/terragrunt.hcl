terraform {
  source = "."
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    flag = "false"
  }
}

# Every parse rejects this config, because exclude reads a dependency output
exclude {
  if      = tostring(dependency.dep.outputs.flag)
  actions = ["all"]
}

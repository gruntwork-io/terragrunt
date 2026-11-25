dependency "base" {
  config_path  = "../base"
  skip_outputs = true

  mock_outputs = {
    id = "mock"
  }
}

inputs = {
  base_id = dependency.base.outputs.id
  stack   = try(path.stack, "no-stack")
}

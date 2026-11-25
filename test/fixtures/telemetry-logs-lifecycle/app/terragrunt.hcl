terraform {
  before_hook "before_apply" {
    commands = ["apply"]
    execute  = ["echo", "before apply"]
  }

  after_hook "after_apply" {
    commands = ["apply"]
    execute  = ["echo", "after apply"]
  }
}

dependency "dep" {
  config_path = "../dep"

  mock_outputs = {
    value = "mock"
  }
}

inputs = {
  value = dependency.dep.outputs.value
}
